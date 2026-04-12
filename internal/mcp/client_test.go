package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTransport is a test transport that returns pre-configured responses.
type mockTransport struct {
	mu        sync.Mutex
	responses map[string]*Response // keyed by method
	calls     []string             // method names called
	closed    bool
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		responses: make(map[string]*Response),
	}
}

func (m *mockTransport) SetResponse(method string, result interface{}) {
	data, _ := json.Marshal(result)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[method] = &Response{
		JSONRPC: jsonRPCVersion,
		Result:  data,
	}
}

func (m *mockTransport) Send(_ context.Context, req *Request) (*Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, req.Method)
	resp, ok := m.responses[req.Method]
	if !ok {
		return nil, fmt.Errorf("no mock response for method %q", req.Method)
	}
	resp.ID = req.ID
	return resp, nil
}

func (m *mockTransport) Notify(_ context.Context, n *Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "notify:"+n.Method)
	return nil
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func TestClient_Initialize(t *testing.T) {
	mt := newMockTransport()
	mt.SetResponse("initialize", InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities:    ServerCaps{Tools: &ToolsCap{}},
		ServerInfo:      ServerInfo{Name: "test-server", Version: "1.0"},
	})

	c := NewClient("test", mt)
	err := c.Initialize(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "test-server", c.ServerName())
	assert.Contains(t, mt.calls, "initialize")
	assert.Contains(t, mt.calls, "notify:notifications/initialized")
}

func TestClient_ListTools(t *testing.T) {
	mt := newMockTransport()
	mt.SetResponse("tools/list", ToolsListResult{
		Tools: []ToolInfo{
			{Name: "search", Description: "Search the web", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "fetch", Description: "Fetch a URL", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	})

	c := NewClient("web", mt)
	tools, err := c.ListTools(context.Background())
	require.NoError(t, err)

	assert.Len(t, tools, 2)
	assert.Equal(t, "search", tools[0].Name)
	assert.Equal(t, "fetch", tools[1].Name)
}

func TestClient_CallTool(t *testing.T) {
	mt := newMockTransport()
	mt.SetResponse("tools/call", ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: "result from server"}},
	})

	c := NewClient("test", mt)
	result, err := c.CallTool(context.Background(), "search", json.RawMessage(`{"query":"hello"}`))
	require.NoError(t, err)

	assert.Len(t, result.Content, 1)
	assert.Equal(t, "result from server", result.Content[0].Text)
	assert.False(t, result.IsError)
}

func TestClient_CallTool_Error(t *testing.T) {
	mt := newMockTransport()
	mt.SetResponse("tools/call", ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: "something went wrong"}},
		IsError: true,
	})

	c := NewClient("test", mt)
	result, err := c.CallTool(context.Background(), "bad-tool", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestClient_Close(t *testing.T) {
	mt := newMockTransport()
	c := NewClient("test", mt)
	err := c.Close()
	require.NoError(t, err)
	assert.True(t, mt.closed)
}

func TestClient_Name(t *testing.T) {
	c := NewClient("my-server", nil)
	assert.Equal(t, "my-server", c.Name())
}

func TestClient_ServerName_BeforeInit(t *testing.T) {
	c := NewClient("fallback", nil)
	assert.Equal(t, "fallback", c.ServerName())
}

func TestRPCError_Error(t *testing.T) {
	e := &RPCError{Code: -32600, Message: "Invalid Request"}
	assert.Equal(t, "Invalid Request", e.Error())
}
