package llm

// NewQwenProvider creates a Provider for Qwen (通义千问) models.
// DashScope exposes an OpenAI-compatible API, so we reuse OpenAIProvider.
func NewQwenProvider(apiKey, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	return NewOpenAIProvider(apiKey, baseURL)
}
