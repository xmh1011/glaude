package llm

import (
	"context"
)

// Provider abstracts LLM communication.
// Different backends (Anthropic, OpenAI, Ollama, Mock) implement this interface.
// Format differences are handled inside each implementation; callers only see unified types.
type Provider interface {
	// Complete sends a completion request and returns the full response.
	Complete(ctx context.Context, req *Request) (*Response, error)
}

// StreamingProvider extends Provider with streaming support.
// Providers that support streaming implement this interface.
type StreamingProvider interface {
	Provider
	// CompleteStream starts a streaming completion. The returned channel
	// delivers events until the stream ends, then is closed.
	// The caller must consume the channel until it closes.
	CompleteStream(ctx context.Context, req *Request) (<-chan StreamEvent, error)
}
