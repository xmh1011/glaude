package llm

import (
	"os"
)

// NewProvider creates a Provider based on the provider name.
// It wraps the base provider with DialectFixer (for non-Anthropic) and RetryProvider.
func NewProvider(providerName, model string) Provider {
	var base Provider
	switch providerName {
	case "openai":
		base = NewOpenAIProvider(os.Getenv("OPENAI_API_KEY"), os.Getenv("OPENAI_BASE_URL"))
	case "ollama":
		base = NewOllamaProvider(os.Getenv("OLLAMA_BASE_URL"))
	default: // "anthropic"
		base = NewAnthropicProvider("")
	}
	// Non-Anthropic backends get DialectFixer for JSON repair
	if providerName != "" && providerName != "anthropic" {
		base = NewDialectFixer(base)
	}
	return NewRetryProvider(base, DefaultRetryConfig())
}
