package llm

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// NewProvider creates a Provider based on the provider name.
// It wraps the base provider with DialectFixer (for non-Anthropic) and RetryProvider.
//
// Configuration is read from viper (config file) first, then falls back to
// environment variables:
//
//	api_key   → ANTHROPIC_API_KEY / OPENAI_API_KEY / GEMINI_API_KEY / ...
//	base_url  → ANTHROPIC_BASE_URL / OPENAI_BASE_URL / ...
func NewProvider(ctx context.Context, providerName string) (Provider, error) {
	var base Provider
	var err error
	switch providerName {
	case "openai":
		apiKey := configOrEnv("api_key", "OPENAI_API_KEY")
		baseURL := configOrEnv("base_url", "OPENAI_BASE_URL")
		base = NewOpenAIProvider(apiKey, baseURL)
	case "qianfan", "oneapi":
		apiKey := configOrEnv("api_key", "QIANFAN_API_KEY")
		baseURL := configOrEnv("base_url", "QIANFAN_BASE_URL")
		base = NewQianfanProvider(apiKey, baseURL)
	case "ollama":
		baseURL := configOrEnv("base_url", "OLLAMA_BASE_URL")
		base = NewOllamaProvider(baseURL)
	case "deepseek":
		apiKey := configOrEnv("api_key", "DEEPSEEK_API_KEY")
		baseURL := configOrEnv("base_url", "DEEPSEEK_BASE_URL")
		base = NewDeepSeekProvider(apiKey, baseURL)
	case "qwen":
		apiKey := configOrEnv("api_key", "DASHSCOPE_API_KEY")
		baseURL := configOrEnv("base_url", "DASHSCOPE_BASE_URL")
		base = NewQwenProvider(apiKey, baseURL)
	case "doubao":
		apiKey := configOrEnv("api_key", "ARK_API_KEY")
		baseURL := configOrEnv("base_url", "ARK_BASE_URL")
		base = NewDoubaoProvider(apiKey, baseURL)
	case "gemini":
		apiKey := configOrEnv("api_key", "GEMINI_API_KEY")
		base, err = NewGeminiProvider(ctx, apiKey)
		if err != nil {
			return nil, fmt.Errorf("gemini provider: %w", err)
		}
	default: // "anthropic"
		apiKey := configOrEnv("api_key", "ANTHROPIC_API_KEY")
		baseURL := configOrEnv("base_url", "ANTHROPIC_BASE_URL")
		base = NewAnthropicProvider(apiKey, baseURL)
	}
	// Non-Anthropic backends get DialectFixer for JSON repair
	if providerName != "" && providerName != "anthropic" {
		base = NewDialectFixer(base)
	}
	return NewRetryProvider(base, DefaultRetryConfig()), nil
}

// configOrEnv reads a value from viper config first; if empty, falls back to
// the given environment variable.
func configOrEnv(viperKey, envVar string) string {
	if v := viper.GetString(viperKey); v != "" {
		return v
	}
	return os.Getenv(envVar)
}
