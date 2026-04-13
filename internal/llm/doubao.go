package llm

// NewDoubaoProvider creates a Provider for Doubao (豆包) models.
// Volcengine ARK exposes an OpenAI-compatible API, so we reuse OpenAIProvider.
func NewDoubaoProvider(apiKey, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://ark.cn-beijing.volces.com/api/v3"
	}
	return NewOpenAIProvider(apiKey, baseURL)
}
