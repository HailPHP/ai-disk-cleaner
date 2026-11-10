package setting

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	settingmodel "ai-disk-cleanner/backend/data/models/setting"
)

type settingStore interface {
	ListSettings(context.Context) ([]settingmodel.Setting, error)
	SaveSettings(context.Context, []settingmodel.Setting) error
}

var supportedSettingKeys = map[string]struct{}{
	"llm.api-mode": {},
	"llm.request-timeout": {},
	"llm.secret":                {},
	"llm.url":                   {},
	"llm.model":                 {},
	"llm.max-token":             {},
	"llm.auto-context-compress": {},
	"llm.extra-body":            {},
	"record.max.count":          {},
}

func validateSettings(settings []settingmodel.Setting) error {
	seen := make(map[string]struct{}, len(settings))
	for _, item := range settings {
		if _, supported := supportedSettingKeys[item.Key]; !supported {
			return fmt.Errorf("unsupported setting key %q", item.Key)
		}
		if _, duplicate := seen[item.Key]; duplicate {
			return fmt.Errorf("duplicate setting key %q", item.Key)
		}
		seen[item.Key] = struct{}{}
		if item.Key == "llm.api-mode" && item.Value != "chat_completions" && item.Value != "responses" {
			return fmt.Errorf("llm.api-mode must be chat_completions or responses")
		}
		if item.Key == "llm.request-timeout" {
			seconds, err := strconv.ParseInt(item.Value, 10, 64)
			if err != nil || seconds < 1 || seconds > 3600 {
				return fmt.Errorf("llm.request-timeout must be between 1 and 3600 seconds")
			}
		}
		if item.Key == "llm.max-token" || item.Key == "record.max.count" {
			value, err := strconv.ParseInt(item.Value, 10, 64)
			if err != nil || value <= 0 {
				return fmt.Errorf("%s must be a positive integer", item.Key)
			}
		}
		if item.Key == "llm.auto-context-compress" {
			if _, err := strconv.ParseBool(item.Value); err != nil {
				return fmt.Errorf("%s must be a boolean", item.Key)
			}
		}
		if item.Key == "llm.extra-body" {
			value := strings.TrimSpace(item.Value)
			if value == "" {
				continue
			}
			var extraBody map[string]any
			if err := json.Unmarshal([]byte(value), &extraBody); err != nil || extraBody == nil {
				return fmt.Errorf("%s must be a JSON object", item.Key)
			}
		}
	}
	// New keys are optional for callers using the pre-migration settings contract.
	for key := range supportedSettingKeys {
		if key == "llm.api-mode" || key == "llm.request-timeout" {
			continue
		}
		if _, ok := seen[key]; !ok {
			return fmt.Errorf("missing setting key %q", key)
		}
	}
	return nil
}

