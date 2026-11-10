package analyzer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
)

type analysisError struct {
	cause   error
	summary string
}

type analysisLogger interface {
	Event(string, string, ...any)
	Round(int, time.Duration, *int64, *int64, *int64, string)
	ToolRequest(int, string, string, string)
}

func completionErrorSummary(err error) string {
	if errors.Is(err, context.Canceled) {
		return "LLM 请求已取消"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "LLM 请求超时"
	}
	var apiError *openai.Error
	if errors.As(err, &apiError) {
		return fmt.Sprintf("LLM 请求失败 HTTP状态=%d", apiError.StatusCode)
	}
	var responseError *invalidResponseError
	if errors.As(err, &responseError) {
		return fmt.Sprintf("LLM 响应格式异常 HTTP状态=%d", responseError.status)
	}
	return "LLM 请求失败，请检查网络、API 模式和服务配置"
}

func (agent *Agent) logCompletion(turn int, elapsed time.Duration, completion *openai.ChatCompletion) {
	var input, output, total *int64
	usage := completion.Usage
	if completion.JSON.Usage.Valid() {
		if usage.JSON.PromptTokens.Valid() && usage.PromptTokens >= 0 {
			input = &usage.PromptTokens
		}
		if usage.JSON.CompletionTokens.Valid() && usage.CompletionTokens >= 0 {
			output = &usage.CompletionTokens
		}
		if usage.JSON.TotalTokens.Valid() && usage.TotalTokens >= 0 {
			total = &usage.TotalTokens
		}
	}
	agent.log.Round(turn, elapsed, input, output, total, "")
}

