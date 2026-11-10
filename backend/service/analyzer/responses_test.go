package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"ai-disk-cleanner/backend/data/models/setting"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"github.com/openai/openai-go/v3/responses"
)

func responseMessage(text string) map[string]any {
	return map[string]any{"id": "msg_test", "type": "message", "role": "assistant", "status": "completed", "phase": "final_answer", "content": []any{
		map[string]any{"type": "output_text", "text": text, "annotations": []any{}},
	}}
}

func responseCall(id, name, arguments string) map[string]any {
	return map[string]any{"id": "fc_" + id, "type": "function_call", "call_id": id, "name": name, "arguments": arguments, "status": "completed"}
}

func writeResponse(t *testing.T, w http.ResponseWriter, output ...any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]any{"id": "resp_test", "object": "response", "status": "completed", "error": nil, "output": output,
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 2, "total_tokens": 12}})
	if err != nil {
		t.Error(err)
	}
}

func readResponseRequest(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	if r.URL.Path != "/v1/responses" || r.Method != http.MethodPost {
		t.Errorf("unexpected endpoint: %s %s", r.Method, r.URL.Path)
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Error(err)
	}
	for _, key := range []string{"store", "stream", "parallel_tool_calls"} {
		if body[key] != false {
			t.Errorf("%s = %v, want false", key, body[key])
		}
	}
	for _, key := range []string{"messages", "previous_response_id", "conversation", "reasoning_effort", "verbosity"} {
		if _, present := body[key]; present {
			t.Errorf("unexpected field: %s", key)
		}
	}
	if !strings.Contains(fmt.Sprint(body["include"]), "reasoning.encrypted_content") {
		t.Error("encrypted reasoning was not requested")
	}
	return body
}

func responsesTestAgent(t *testing.T, ctx context.Context, url string) *Agent {
	t.Helper()
	tree := &modelscanner.FileTree{Root: &modelscanner.FileNode{Name: "root", Type: modelscanner.NodeTypeDirectory, DiskSize: 42}}
	agent, err := newAgent(ctx, tree, "KEEP_SYSTEM_INSTRUCTIONS", nil, &llmConfig{
		apiMode: apiModeResponses, baseURL: url + "/v1", secret: "PRIVATE_KEY", model: "gpt-6-astra", maxTokens: 100000,
		autoContextCompressEnabled: true,
	}, "en")
	if err != nil {
		t.Fatal(err)
	}
	return agent
}

func TestResponsesAgentReplaysItemsAndExecutesOriginalTools(t *testing.T) {
	requests := 0
	reasoning := map[string]any{"id": "rs_1", "type": "reasoning", "summary": []any{}, "encrypted_content": "PRIVATE_ENCRYPTED", "future_field": "preserve_me"}
	commentary := responseMessage("Analyzing. ")
	commentary["phase"] = "commentary"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body := readResponseRequest(t, r)
		if body["instructions"] != "KEEP_SYSTEM_INSTRUCTIONS" || body["model"] != "gpt-6-astra" || body["tool_choice"] != "auto" {
			t.Error("model, instructions or tool choice lost")
		}
		if body["reasoning"].(map[string]any)["effort"] != "low" || body["text"].(map[string]any)["verbosity"] != "medium" {
			t.Error("Astra defaults missing")
		}
		input := body["input"].([]any)
		switch requests {
		case 1:
			tools := body["tools"].([]any)
			if len(tools) != 4 { t.Errorf("tools = %d", len(tools)) }
			for _, raw := range tools {
				tool := raw.(map[string]any)
				if tool["type"] != "function" || tool["strict"] != true || tool["parameters"] == nil || tool["function"] != nil {
					t.Errorf("incorrect Responses tool shape: %v", tool)
				}
			}
			writeResponse(t, w, reasoning, commentary, responseCall("call_scan", "analyze_directory", `{"path":"/"}`))
		case 2:
			if len(input) != 5 || !reflect.DeepEqual(input[1], reasoning) || !reflect.DeepEqual(input[2], commentary) {
				t.Error("full output item replay/order was lost")
			}
			result := input[len(input)-1].(map[string]any)
			if result["call_id"] != "call_scan" || result["type"] != "function_call_output" || !strings.Contains(result["output"].(string), "/,42,0") {
				t.Errorf("scan result not correlated: %v", result)
			}
			writeResponse(t, w, responseCall("call_trash", "add_trash_file", `{"files":[{"name":"cache","reason":"test","path":"/cache","size":10,"level":0,"isDeleted":false}]}`), responseCall("call_usage", "add_top_usages", `{"usages":[{"path":"/","size":42,"description":"test"}]}`))
		case 3:
			for i, id := range []string{"call_trash", "call_usage"} {
				result := input[len(input)-2+i].(map[string]any)
				if result["call_id"] != id || result["output"] != "true" { t.Errorf("result=%v", result) }
			}
			writeResponse(t, w, responseCall("call_compress", "compress_context", `{"summary":"Scanned /; do not rescan. Remaining: none."}`))
		case 4:
			encoded, _ := json.Marshal(input)
			if len(input) != 1 || !strings.Contains(string(encoded), "Scanned /") || strings.Contains(string(encoded), "PRIVATE_ENCRYPTED") || strings.Contains(string(encoded), "function_call_output") {
				t.Error("compression failed to replace history with summary")
			}
			writeResponse(t, w, responseMessage("Done."))
		default:
			t.Error("unexpected extra request")
			writeResponse(t, w, responseMessage("stop"))
		}
	}))
	defer server.Close()
	agent := responsesTestAgent(t, context.Background(), server.URL)
	recorder := &logRecorder{}
	agent.log = recorder
	var visible strings.Builder
	agent.onDelta = func(text string) { visible.WriteString(text) }
	result, err := agent.run()
	if err != nil { t.Fatal(err) }
	if requests != 4 || result.TokenUsage != 48 || result.LLMOutput != "Analyzing. Done." || visible.String() != result.LLMOutput {
		t.Fatalf("requests=%d result=%+v", requests, result)
	}
	if len(result.TrashFiles) != 1 || result.TrashFiles[0].Path != "/cache" || len(result.TopUsages) != 1 || result.TopUsages[0].Size != 42 {
		t.Fatalf("tools did not update analysis results: %+v", result)
	}
	if len(recorder.rounds) != 4 || len(recorder.tools) != 4 || agent.contextRevision != 1 { t.Fatalf("log=%+v", recorder) }
	for _, round := range recorder.rounds {
		if round.input == nil || *round.input != 10 || round.output == nil || *round.output != 2 || round.total == nil || *round.total != 12 { t.Errorf("usage=%+v", round) }
	}
	if strings.Contains(fmt.Sprint(recorder), "PRIVATE_") { t.Fatal("private response/credential in logs") }
}

func TestResponsesCompressionFailurePreservesCallResultPairs(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprint(mixed), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				body := readResponseRequest(t, r)
				if requests == 1 {
					if mixed {
						writeResponse(t, w, responseCall("compress", "compress_context", `{"summary":"valid"}`), responseCall("usage", "add_top_usages", `{"usages":[]}`))
					} else {
						writeResponse(t, w, responseCall("compress", "compress_context", `{"summary":""}`))
					}
					return
				}
				input := body["input"].([]any)
				found := false
				for _, raw := range input {
					item := raw.(map[string]any)
					if item["type"] == "function_call_output" && item["call_id"] == "compress" && item["output"] != "Ok" { found = true }
				}
				if !found { t.Error("failed compression left an orphan call") }
				writeResponse(t, w, responseMessage("done"))
			}))
			defer server.Close()
			agent := responsesTestAgent(t, context.Background(), server.URL)
			if _, err := agent.run(); err != nil { t.Fatal(err) }
			if agent.contextRevision != 0 { t.Fatal("failed compression reset history") }
		})
	}
}

func TestResponsesRejectsIncompleteMalformedAndUnsafeOutputs(t *testing.T) {
	for _, test := range []struct { name, body string; status int }{
		{"http_error", `{"error":{"message":"PRIVATE_KEY PRIVATE_BODY"}}`, 400},
		{"failed", `{"status":"failed","error":{"message":"PRIVATE_BODY"}}`, 200},
		{"incomplete", `{"status":"incomplete","output":[{"type":"function_call","name":"add_top_usages","call_id":"one","arguments":"{\"usages\":[]}"}]}`, 200},
		{"empty", `{"status":"completed","output":[]}`, 200},
		{"missing_status", `{"output":[]}`, 200},
		{"refusal", `{"status":"completed","output":[{"type":"message","content":[{"type":"refusal","refusal":"PRIVATE_BODY"}]}]}`, 200},
		{"missing_id", `{"status":"completed","output":[{"type":"function_call","name":"add_top_usages","arguments":"{}"}]}`, 200},
		{"duplicate_id", `{"status":"completed","output":[{"type":"function_call","call_id":"one","name":"add_top_usages","arguments":"{}"},{"type":"function_call","call_id":"one","name":"add_top_usages","arguments":"{}"}]}`, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			agent := responsesTestAgent(t, context.Background(), server.URL)
			recorder := &logRecorder{}
			agent.log = recorder
			_, err := agent.run()
			if err == nil { t.Fatal("expected failure") }
			if strings.Contains(err.Error(), "PRIVATE_") || strings.Contains(fmt.Sprint(recorder), "PRIVATE_") { t.Fatal("sensitive failure exposed") }
			if len(recorder.tools) != 0 { t.Fatal("invalid response invoked tools") }
		})
	}
}

func TestResponsesCancellationAndBudget(t *testing.T) {
	t.Run("in_flight_cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cancel()
			<-r.Context().Done()
		}))
		defer server.Close()
		agent := responsesTestAgent(t, ctx, server.URL)
		_, err := agent.run()
		if !errors.Is(err, context.Canceled) { t.Fatalf("error=%v", err) }
	})
	t.Run("budget", func(t *testing.T) {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			writeResponse(t, w, responseCall("first", "add_top_usages", `{"usages":[]}`))
		}))
		defer server.Close()
		agent := responsesTestAgent(t, context.Background(), server.URL)
		agent.config.maxTokens = 12
		if _, err := agent.run(); err == nil || requests != 1 { t.Fatalf("requests=%d error=%v", requests, err) }
	})
}

func testLLMSettings(url, mode string) []setting.Setting {
	return []setting.Setting{{Key:"llm.secret", Value:"PRIVATE_KEY"}, {Key:"llm.url", Value:url}, {Key:"llm.model", Value:"gpt-6-astra"}, {Key:"llm.max-token", Value:"10000"}, {Key:"llm.api-mode", Value:mode}}
}

func TestConnectionUsesSelectedAPI(t *testing.T) {
	for _, mode := range []string{apiModeChatCompletions, apiModeResponses} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == apiModeResponses {
					body := readResponseRequest(t, r)
					if body["tools"] != nil || body["max_output_tokens"] != float64(1024) { t.Error("incorrect connection probe") }
					writeResponse(t, w, responseMessage("ok"))
				} else {
					if r.URL.Path != "/v1/chat/completions" { t.Error(r.URL.Path) }
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
				}
			}))
			defer server.Close()
			result, err := newService(nil).TestConnection(context.Background(), testLLMSettings(server.URL+"/v1", mode))
			if err != nil || result.Type != "success" { t.Fatalf("result=%+v error=%v", result, err) }
		})
	}
}

func TestConfigDefaultsAndResponsesGuards(t *testing.T) {
	config, err := parseLLMConfig(testLLMSettings("https://example.com/v1", ""))
	if err != nil || config.apiMode != apiModeChatCompletions || config.requestTimeout != 120*time.Second { t.Fatalf("config=%+v error=%v", config, err) }
	for _, body := range []string{`{"store":true}`, `{"parallel_tool_calls":true}`, `{"stream":true}`, `{"input":[]}`, `{"tools":[]}`, `{"previous_response_id":"old"}`, `{"reasoning_effort":"low"}`, `{"verbosity":"medium"}`, `{"include":123}`} {
		settings := append(testLLMSettings("https://example.com/v1", apiModeResponses), setting.Setting{Key:"llm.extra-body", Value:body})
		if _, err := parseLLMConfig(settings); err == nil { t.Errorf("accepted unsafe/incompatible extra body: %s", body) }
	}
	settings := append(testLLMSettings("https://example.com/v1", apiModeResponses), setting.Setting{Key:"llm.request-timeout",Value:"300"}, setting.Setting{Key:"llm.extra-body", Value:`{"reasoning":{"effort":"high"},"text":{"verbosity":"low"},"parallel_tool_calls":false}`})
	config, err = parseLLMConfig(settings)
	if err != nil || config.requestTimeout != 300*time.Second { t.Fatalf("error=%v", err) }
	encoded, err := json.Marshal(newResponsesParams(config, "system", nil))
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(encoded), `"effort":"high"`) || !strings.Contains(string(encoded), `"verbosity":"low"`) { t.Fatal(string(encoded)) }
}

func TestResponsesUsagePresence(t *testing.T) {
	for _, test := range []struct{body string; input, output, total bool}{
		{`{}`,false,false,false}, {`{"usage":null}`,false,false,false},
		{`{"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`,true,true,true},
		{`{"usage":{"total_tokens":12}}`,false,false,true},
		{`{"usage":{"total_tokens":-1}}`,false,false,false},
	} {
		var response responses.Response
		if err := json.Unmarshal([]byte(test.body), &response); err != nil { t.Fatal(err) }
		recorder := &logRecorder{}
		agent := &Agent{log:recorder}
		agent.logResponse(1, 0, &response, "")
		round := recorder.rounds[0]
		if (round.input != nil) != test.input || (round.output != nil) != test.output || (round.total != nil) != test.total { t.Fatalf("round=%+v", round) }
	}
}
