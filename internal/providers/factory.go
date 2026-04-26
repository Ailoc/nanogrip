package providers

import (
	"fmt"
	"os"
	"strings"
)

// ProviderName identifies the supported LLM API providers.
type ProviderName string

const (
	ProviderOpenAI    ProviderName = "openai"
	ProviderAnthropic ProviderName = "anthropic"
)

// APIConfig contains authentication and optional endpoint settings for one provider.
type APIConfig struct {
	APIKey  string
	APIBase string
}

// ProviderOptions contains all provider settings needed to create an LLMProvider.
type ProviderOptions struct {
	DefaultModel string
	OpenAI       APIConfig
	Anthropic    APIConfig
}

// NewProvider creates the only supported provider for the configured default model.
func NewProvider(opts ProviderOptions) (LLMProvider, error) {
	providerName, _, err := ResolveModel(opts.DefaultModel)
	if err != nil {
		return nil, err
	}

	switch providerName {
	case ProviderOpenAI:
		if !hasCredential(opts.OpenAI, "OPENAI_API_KEY") {
			return nil, fmt.Errorf("openai model %q requires providers.openai.apiKey or OPENAI_API_KEY", opts.DefaultModel)
		}
		return NewOpenAIProvider(opts.OpenAI.APIKey, opts.OpenAI.APIBase, opts.DefaultModel), nil
	case ProviderAnthropic:
		if !hasCredential(opts.Anthropic, "ANTHROPIC_API_KEY") {
			return nil, fmt.Errorf("anthropic model %q requires providers.anthropic.apiKey or ANTHROPIC_API_KEY", opts.DefaultModel)
		}
		return NewAnthropicProvider(opts.Anthropic.APIKey, opts.Anthropic.APIBase, opts.DefaultModel), nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", providerName)
	}
}

// ResolveModel validates a model name and returns its provider plus API model name.
func ResolveModel(model string) (ProviderName, string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", "", fmt.Errorf("model is required")
	}

	if prefix, name, ok := splitModelPrefix(model); ok {
		switch normalizeProviderName(prefix) {
		case string(ProviderOpenAI):
			if name == "" {
				return "", "", fmt.Errorf("openai model name is required")
			}
			return ProviderOpenAI, name, nil
		case string(ProviderAnthropic):
			if name == "" {
				return "", "", fmt.Errorf("anthropic model name is required")
			}
			return ProviderAnthropic, name, nil
		default:
			return "", "", fmt.Errorf("unsupported provider prefix %q: only openai/ and anthropic/ are supported", prefix)
		}
	}

	lower := strings.ToLower(model)
	if strings.HasPrefix(lower, "claude") {
		return ProviderAnthropic, model, nil
	}
	if isOpenAIModel(lower) {
		return ProviderOpenAI, model, nil
	}

	return "", "", fmt.Errorf("unsupported model %q: use openai/<model> or anthropic/<model>", model)
}

func normalizeModelForProvider(providerName ProviderName, model string, defaultModel string) (string, error) {
	if strings.TrimSpace(model) == "" {
		model = defaultModel
	}

	resolvedProvider, apiModel, err := ResolveModel(model)
	if err != nil {
		return "", err
	}
	if resolvedProvider != providerName {
		return "", fmt.Errorf("model %q belongs to %s, but this provider is %s", model, resolvedProvider, providerName)
	}
	return apiModel, nil
}

func splitModelPrefix(model string) (string, string, bool) {
	prefix, name, ok := strings.Cut(model, "/")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(prefix), strings.TrimSpace(name), true
}

func normalizeProviderName(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "-", "_")
}

func isOpenAIModel(model string) bool {
	for _, prefix := range []string{"gpt-", "o1", "o3", "o4", "chatgpt-", "codex-"} {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

func hasCredential(cfg APIConfig, envKey string) bool {
	if strings.TrimSpace(cfg.APIKey) != "" {
		return true
	}
	_, ok := os.LookupEnv(envKey)
	return ok
}
