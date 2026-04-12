package llm

import (
	"os"

	"github.com/spf13/viper"
)

// NewProvider creates a Provider based on the provider name.
// It wraps the base provider with DialectFixer (for non-Anthropic) and RetryProvider.
//
// Configuration is read from viper (config file) first, then falls back to
// environment variables:
//
//	api_key   → ANTHROPIC_API_KEY / OPENAI_API_KEY
//	base_url  → ANTHROPIC_BASE_URL / OPENAI_BASE_URL / OLLAMA_BASE_URL
func NewProvider(providerName, model string) Provider {
	var base Provider
	switch providerName {
	case "openai", "oneapi":
		apiKey := configOrEnv("api_key", "OPENAI_API_KEY")
		baseURL := configOrEnv("base_url", "OPENAI_BASE_URL")
		base = NewOpenAIProvider(apiKey, baseURL)
	case "ollama":
		baseURL := configOrEnv("base_url", "OLLAMA_BASE_URL")
		base = NewOllamaProvider(baseURL)
	default: // "anthropic"
		apiKey := configOrEnv("api_key", "ANTHROPIC_API_KEY")
		baseURL := configOrEnv("base_url", "ANTHROPIC_BASE_URL")
		base = NewAnthropicProvider(apiKey, baseURL)
	}
	// Non-Anthropic backends get DialectFixer for JSON repair
	if providerName != "" && providerName != "anthropic" {
		base = NewDialectFixer(base)
	}
	return NewRetryProvider(base, DefaultRetryConfig())
}

// configOrEnv reads a value from viper config first; if empty, falls back to
// the given environment variable.
func configOrEnv(viperKey, envVar string) string {
	if v := viper.GetString(viperKey); v != "" {
		return v
	}
	return os.Getenv(envVar)
}
