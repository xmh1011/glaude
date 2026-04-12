package llm

import (
	"context"
)

// Provider abstracts LLM communication.
// Different backends (Anthropic, OpenAI, Ollama, Mock) implement this interface.
// Format differences are handled inside each implementation; callers only see unified types.
type Provider interface {
	// Complete sends a completion request and returns the full response.
	// Streaming will be added in Phase 10.
	Complete(ctx context.Context, req *Request) (*Response, error)
}
