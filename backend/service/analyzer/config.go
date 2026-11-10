package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ai-disk-cleanner/backend/data/models/setting"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type settingStore interface {
	ListSettings(context.Context) ([]setting.Setting, error)
}

type llmConfig struct {
	apiMode                    string
	requestTimeout             time.Duration
	secret                     string
	baseURL                    string
	model                      string
	maxTokens                  int64
	autoContextCompressEnabled bool
	extraBody                  map[string]any
}

const (
	apiModeChatCompletions = "chat_completions"
	apiModeResponses = "responses"
)

func (analyzer *Service) loadLLMConfig(ctx context.Context) (*llmConfig, error) {
	if analyzer.settings == nil {
		return nil, errors.New("load LLM configuration: setting store is nil")
	}
	settings, err := analyzer.settings.ListSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load LLM configuration: %w", err)
	}
	return parseLLMConfig(settings)
}

func parseLLMConfig(settings []setting.Setting) (*llmConfig, error) {
	var err error
	values := make(map[string]string, len(settings))
	for _, item := range settings {
		values[item.Key] = item.Value
	}

	config := llmConfig{
		apiMode: strings.TrimSpace(values["llm.api-mode"]),
		requestTimeout: 120 * time.Second,
		secret:  strings.TrimSpace(values["llm.secret"]),
		baseURL: strings.TrimSpace(values["llm.url"]),
		model:   strings.TrimSpace(values["llm.model"]),
	}
	if config.apiMode == "" {
		config.apiMode = apiModeChatCompletions
	}
	if config.apiMode != apiModeChatCompletions && config.apiMode != apiModeResponses {
		return nil, errors.New("load LLM configuration: llm.api-mode must be chat_completions or responses")
	}
	if timeout := strings.TrimSpace(values["llm.request-timeout"]); timeout != "" {
		seconds, err := strconv.ParseInt(timeout, 10, 64)
		if err != nil || seconds < 1 || seconds > 3600 {
			return nil, errors.New("load LLM configuration: llm.request-timeout must be between 1 and 3600 seconds")
		}
		config.requestTimeout = time.Duration(seconds) * time.Second
	}
	if config.secret == "" {
		return nil, errors.New("load LLM configuration: llm.secret is empty")
	}
	if config.baseURL == "" {
		return nil, errors.New("load LLM configuration: llm.url is empty")
	}
	if config.model == "" {
		return nil, errors.New("load LLM configuration: llm.model is empty")
	}
	config.maxTokens, err = strconv.ParseInt(strings.TrimSpace(values["llm.max-token"]), 10, 64)
	if err != nil || config.maxTokens <= 0 {
		return nil, errors.New("load LLM configuration: llm.max-token must be a positive integer")
	}
	autoContextCompress := strings.TrimSpace(values["llm.auto-context-compress"])
	if autoContextCompress != "" {
		config.autoContextCompressEnabled, err = strconv.ParseBool(autoContextCompress)
		if err != nil {
			return nil, errors.New("load LLM configuration: llm.auto-context-compress must be a boolean")
		}
	}
	extraBody := strings.TrimSpace(values["llm.extra-body"])
	if extraBody != "" {
		if err := json.Unmarshal([]byte(extraBody), &config.extraBody); err != nil || config.extraBody == nil {
			return nil, errors.New("load LLM configuration: llm.extra-body must be a JSON object")
		}
	}
	if config.apiMode == apiModeResponses {
		if err := validateResponsesExtraBody(config.extraBody); err != nil {
			return nil, err
		}
	}
	return &config, nil
}

type debugTransport struct {
	rt http.RoundTripper
}

type invalidResponseError struct {
	status int
}

func (t debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.rt.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-Type")

	if resp.StatusCode >= 400 ||
		!strings.Contains(contentType, "application/json") {

		// Do not retain or expose upstream bodies: gateways can echo credentials.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()

		return nil, &invalidResponseError{status: resp.StatusCode}
	}

	return resp, nil
}
func newOpenAIClient(config *llmConfig) openai.Client {
	timeout := config.requestTimeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	client := &http.Client{
		Transport: debugTransport{
			rt: http.DefaultTransport,
		},
	}
	return openai.NewClient(
		option.WithAPIKey(config.secret),
		option.WithBaseURL(config.baseURL),
		option.WithRequestTimeout(timeout),
		//option.WithDebugLog(log.New(
		//	os.Stdout,
		//	"[openai] ",
		//	log.LstdFlags,
		//)),
		option.WithHTTPClient(client),
	)
}

