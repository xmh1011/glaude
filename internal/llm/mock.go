package llm

import (
	"context"
	"fmt"
)

// MockProvider implements Provider with pre-scripted responses.
// Each call to Complete returns the next response in sequence.
// Useful for testing the Agent loop without any API dependency.
type MockProvider struct {
	Responses []*Response
	// Requests records every request received, for test assertions.
	Requests []*Request
	index    int
}

// NewMockProvider creates a MockProvider with the given scripted responses.
func NewMockProvider(responses ...*Response) *MockProvider {
	return &MockProvider{Responses: responses}
}

// Complete returns the next pre-scripted response.
func (m *MockProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	m.Requests = append(m.Requests, req)

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.index >= len(m.Responses) {
		return nil, fmt.Errorf("mock: no more responses (used %d/%d)", m.index, len(m.Responses))
	}

	resp := m.Responses[m.index]
	m.index++
	return resp, nil
}

// CompleteStream synthesizes a stream of events from the next scripted response.
// Text blocks become text_delta events; tool_use blocks become start + json_delta + stop events.
func (m *MockProvider) CompleteStream(ctx context.Context, req *Request) (<-chan StreamEvent, error) {
	m.Requests = append(m.Requests, req)

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.index >= len(m.Responses) {
		return nil, fmt.Errorf("mock: no more responses (used %d/%d)", m.index, len(m.Responses))
	}

	resp := m.Responses[m.index]
	m.index++

	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)

		for i, cb := range resp.Content {
			if ctx.Err() != nil {
				return
			}
			switch cb.Type {
			case ContentText:
				// Send text as a single delta
				ch <- StreamEvent{
					Type:  EventTextDelta,
					Text:  cb.Text,
					Index: i,
				}
				ch <- StreamEvent{
					Type:  EventContentBlockStop,
					Index: i,
				}
			case ContentToolUse:
				ch <- StreamEvent{
					Type:  EventToolUseStart,
					ID:    cb.ID,
					Name:  cb.Name,
					Index: i,
				}
				if len(cb.Input) > 0 {
					ch <- StreamEvent{
						Type:      EventInputJSONDelta,
						InputJSON: string(cb.Input),
						Index:     i,
					}
				}
				ch <- StreamEvent{
					Type:  EventContentBlockStop,
					Index: i,
				}
			}
		}

		ch <- StreamEvent{
			Type:       EventMessageDelta,
			StopReason: resp.StopReason,
			Usage:      resp.Usage,
		}
	}()

	return ch, nil
}

// Compile-time interface checks.
var (
	_ Provider          = (*MockProvider)(nil)
	_ StreamingProvider = (*MockProvider)(nil)
)

// MockStreamEvents is a helper for tests that need custom event sequences.
type MockStreamEvents struct {
	Events []StreamEvent
}

// CompleteStream sends the pre-built events on a channel.
func (m *MockStreamEvents) CompleteStream(ctx context.Context, req *Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, len(m.Events))
	go func() {
		defer close(ch)
		for _, e := range m.Events {
			if ctx.Err() != nil {
				return
			}
			ch <- e
		}
	}()
	return ch, nil
}

// Complete is not supported on MockStreamEvents; it panics.
func (m *MockStreamEvents) Complete(ctx context.Context, req *Request) (*Response, error) {
	return nil, fmt.Errorf("MockStreamEvents does not support Complete()")
}
