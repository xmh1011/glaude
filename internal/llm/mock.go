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
