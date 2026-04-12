package llm

// NewOllamaProvider creates a Provider for local Ollama models.
// Ollama exposes an OpenAI-compatible /v1/chat/completions endpoint,
// so we reuse OpenAIProvider with a different base URL.
func NewOllamaProvider(baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434/v1"
	}
	// Ollama doesn't require a real API key
	return NewOpenAIProvider("ollama", baseURL)
}
