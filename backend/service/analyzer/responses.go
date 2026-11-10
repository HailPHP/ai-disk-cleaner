package analyzer

import (
	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

// The agent owns its history and tool catalog. Extra Body may tune the model,
// but must not replace the protocol or enable server-side conversation storage.
func validateResponsesExtraBody(extra map[string]any) error {
	for key, value := range extra {
		switch key {
		case "reasoning_effort", "verbosity", "messages", "max_tokens", "max_completion_tokens":
			return fmt.Errorf("Responses API: Extra Body field %q is incompatible; use reasoning.effort, text.verbosity and max_output_tokens", key)
		case "input", "instructions", "model", "tools", "previous_response_id", "conversation", "context_management", "stream_options":
			return fmt.Errorf("Responses API: Extra Body field %q is managed by the application", key)
		case "store", "stream", "parallel_tool_calls", "background":
			if flag, ok := value.(bool); !ok || flag {
				return fmt.Errorf("Responses API: %s must be false", key)
			}
		case "include":
			items, ok := value.([]any)
			if !ok {
				return errors.New("Responses API: include must be an array of strings")
			}
			for _, item := range items {
				if _, ok := item.(string); !ok {
					return errors.New("Responses API: include must be an array of strings")
				}
			}
		}
	}
	return nil
}

func newResponsesParams(config *llmConfig, instructions string, input []responses.ResponseInputItemUnionParam) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model: config.model,
		Instructions: openai.String(instructions),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: input},
		MaxOutputTokens: openai.Int(8000),
		Store: openai.Bool(false),
		ParallelToolCalls: openai.Bool(false),
		Include: []responses.ResponseIncludable{"reasoning.encrypted_content"},
	}
	extra := make(map[string]any, len(config.extraBody)+2)
	for key, value := range config.extraBody {
		extra[key] = value
	}
	// Astra defaults follow the documented configuration; other providers/models
	// keep their own defaults unless explicitly configured in Extra Body.
	if config.model == "gpt-6-astra" || strings.HasPrefix(config.model, "gpt-6-astra-") {
		for key, defaults := range map[string]map[string]any{
			"reasoning": {"effort": "low"},
			"text": {"verbosity": "medium"},
		} {
			if existing, ok := extra[key].(map[string]any); ok {
				for field, value := range existing {
					defaults[field] = value
				}
			} else if _, present := extra[key]; present {
				continue
			}
			extra[key] = defaults
		}
	}
	include := []string{"reasoning.encrypted_content"}
	if items, ok := extra["include"].([]any); ok {
		for _, item := range items {
			if value, ok := item.(string); ok && value != "reasoning.encrypted_content" {
				include = append(include, value)
			}
		}
	}
	extra["include"] = include
	extra["stream"] = false
	extra["store"] = false
	extra["parallel_tool_calls"] = false
	if _, ok := extra["tool_choice"]; !ok {
		extra["tool_choice"] = "auto"
	}
	params.SetExtraFields(extra)
	return params
}

func buildResponsesTools(manager *toolsManager, agent *Agent) []responses.ToolUnionParam {
	result := make([]responses.ToolUnionParam, 0, len(manager.tools))
	for _, tool := range manager.tools {
		if tool.IsSupport(agent) {
			result = append(result, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{
				Name: tool.Name(),
				Description: openai.String(tool.Description()),
				Parameters: tool.ParameterSchema(),
				Strict: openai.Bool(true),
			}})
		}
	}
	return result
}

func responseStatusError(response *responses.Response) error {
	if response.Status == "incomplete" {
		return errors.New("LLM 响应未完成，请检查 max_output_tokens 或降低推理强度")
	}
	if response.Status != "completed" || response.JSON.Error.Valid() {
		return errors.New("LLM 响应失败或状态异常，请检查模型与 API 配置")
	}
	for _, item := range response.Output {
		for _, content := range item.Content {
			if content.Type == "refusal" {
				return errors.New("LLM 拒绝了本次请求")
			}
		}
	}
	return nil
}

func (agent *Agent) runResponses() (*cleaningrecord.AnalysisResult, error) {
	if err := validateResponsesExtraBody(agent.config.extraBody); err != nil {
		return nil, err
	}
	client := newOpenAIClient(agent.config)
	manager := newManager()
	seenCalls := make(map[string]bool)
	var output strings.Builder
	for turn := 1; ; turn++ {
		if err := agent.ctx.Err(); err != nil {
			return nil, err
		}
		if err := agent.beforeCompletions(); err != nil {
			return nil, err
		}
		params := newResponsesParams(agent.config, agent.systemPrompt, agent.responsesInput)
		params.Tools = buildResponsesTools(manager, agent)
		started := time.Now()
		response, err := client.Responses.New(agent.ctx, params)
		if err != nil {
			summary := completionErrorSummary(err)
			agent.log.Round(turn, time.Since(started), nil, nil, nil, summary)
			return nil, &analysisError{cause: err, summary: summary}
		}
		statusErr := responseStatusError(response)
		summary := ""
		if statusErr != nil {
			summary = statusErr.Error()
		}
		agent.logResponse(turn, time.Since(started), response, summary)
		if response.Usage.JSON.TotalTokens.Valid() && response.Usage.TotalTokens >= 0 {
			agent.usedTokens += response.Usage.TotalTokens
			agent.totalTokens = response.Usage.TotalTokens
		}
		agent.afterCompletions()
		if err := agent.ctx.Err(); err != nil {
			return nil, err
		}
		if statusErr != nil {
			return nil, statusErr
		}

		calls := make([]responses.ResponseFunctionToolCall, 0)
		// Validate the entire batch before any local tool can change agent state.
		for _, item := range response.Output {
			if item.Type != "function_call" {
				continue
			}
			call := item.AsFunctionCall()
			if call.CallID == "" || call.Name == "" || seenCalls[call.CallID] || (call.Status != "" && call.Status != "completed") {
				return nil, errors.New("LLM 返回无效或重复的工具调用")
			}
			seenCalls[call.CallID] = true
			calls = append(calls, call)
		}
		for _, item := range response.Output {
			// Preserve all original fields, including encrypted reasoning, message
			// phases and fields introduced after this SDK version.
			agent.responsesInput = append(agent.responsesInput,
				param.Override[responses.ResponseInputItemUnionParam](json.RawMessage(item.RawJSON())))
		}
		text := response.OutputText()
		if text != "" {
			output.WriteString(text)
			agent.onDelta(text)
		}
		if len(calls) == 0 {
			if strings.TrimSpace(text) == "" {
				return nil, errors.New("LLM 未返回文本或工具调用")
			}
			break
		}
		for _, call := range calls {
			if err := agent.ctx.Err(); err != nil {
				return nil, err
			}
			agent.log.ToolRequest(turn, call.CallID, call.Name, call.Arguments)
			revision := agent.contextRevision
			var result string
			var toolErr error
			tool, exists := manager.toolMap[call.Name]
			switch {
			case !exists:
				toolErr = errors.New("unknown tool")
			case !tool.IsSupport(agent):
				toolErr = errors.New("tool is unavailable in the current context state")
			case call.Name == compressContextToolName && len(calls) != 1:
				// A mixed batch cannot safely describe results from sibling calls in
				// its summary. Keep all call/result pairs and ask for a separate call.
				toolErr = errors.New("compress_context must be called alone")
			default:
				result, toolErr = manager.Invoke(call.Name, call.Arguments, agent)
			}
			if toolErr != nil {
				result = toolErr.Error()
			}
			if revision == agent.contextRevision {
				agent.responsesInput = append(agent.responsesInput,
					responses.ResponseInputItemParamOfFunctionCallOutput(call.CallID, result))
			}
		}
	}
	return &cleaningrecord.AnalysisResult{
		TrashFiles: agent.TrashFiles,
		TopUsages: agent.TopUsages,
		LLMOutput: output.String(),
		TokenUsage: agent.usedTokens,
	}, nil
}

func testResponsesConnection(ctx context.Context, config *llmConfig) (TestConnectionResult, error) {
	probeConfig := *config
	probeConfig.extraBody = make(map[string]any, len(config.extraBody)+2)
	for key, value := range config.extraBody {
		probeConfig.extraBody[key] = value
	}
	// Connection tests are plain-text probes; agent-only tool choices are ignored.
	probeConfig.extraBody["tool_choice"] = "auto"
	probeConfig.extraBody["max_output_tokens"] = 1024
	params := newResponsesParams(&probeConfig, "", []responses.ResponseInputItemUnionParam{
		responses.ResponseInputItemParamOfMessage("reply ok only", responses.EasyInputMessageRoleUser),
	})
	client := newOpenAIClient(config)
	response, err := client.Responses.New(ctx, params)
	if err != nil {
		return TestConnectionResult{}, &analysisError{cause: err, summary: completionErrorSummary(err)}
	}
	if err := responseStatusError(response); err != nil {
		return TestConnectionResult{}, err
	}
	if strings.TrimSpace(response.OutputText()) == "" {
		return TestConnectionResult{}, errors.New("LLM 未返回测试文本")
	}
	return TestConnectionResult{Type: "success"}, nil
}

func (agent *Agent) logResponse(turn int, elapsed time.Duration, response *responses.Response, summary string) {
	var input, output, total *int64
	usage := response.Usage
	if response.JSON.Usage.Valid() {
		if usage.JSON.InputTokens.Valid() && usage.InputTokens >= 0 {
			input = &usage.InputTokens
		}
		if usage.JSON.OutputTokens.Valid() && usage.OutputTokens >= 0 {
			output = &usage.OutputTokens
		}
		if usage.JSON.TotalTokens.Valid() && usage.TotalTokens >= 0 {
			total = &usage.TotalTokens
		}
	}
	agent.log.Round(turn, elapsed, input, output, total, summary)
}
