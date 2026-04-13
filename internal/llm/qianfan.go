package llm

// NewQianfanProvider creates a Provider for Baidu Qianfan (百度千帆) models.
// Qianfan exposes an OpenAI-compatible API, so we reuse OpenAIProvider.
func NewQianfanProvider(apiKey, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://qianfan.baidubce.com/v2"
	}
	return NewOpenAIProvider(apiKey, baseURL)
}
