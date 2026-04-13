package llm

// NewDeepSeekProvider creates a Provider for DeepSeek models.
// DeepSeek exposes an OpenAI-compatible API, so we reuse OpenAIProvider.
func NewDeepSeekProvider(apiKey, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.deepseek.com/v1"
	}
	return NewOpenAIProvider(apiKey, baseURL)
}
