package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPTool_Name(t *testing.T) {
	client := NewClient("github", nil)
	tool := NewMCPTool(client, ToolInfo{
		Name:        "search_repos",
		Description: "Search GitHub repositories",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	})
	assert.Equal(t, "mcp__github__search_repos", tool.Name())
}

func TestMCPTool_Description(t *testing.T) {
	client := NewClient("server1", nil)
	tool := NewMCPTool(client, ToolInfo{
		Name:        "hello",
		Description: "Says hello",
	})
	assert.Equal(t, "Says hello", tool.Description())
}

func TestMCPTool_IsReadOnly(t *testing.T) {
	client := NewClient("server1", nil)
	tool := NewMCPTool(client, ToolInfo{Name: "test"})
	assert.False(t, tool.IsReadOnly())
}

func TestMCPTool_Execute_Success(t *testing.T) {
	mt := newMockTransport()
	mt.SetResponse("tools/call", ToolCallResult{
		Content: []ToolContent{
			{Type: "text", Text: "Hello from MCP!"},
		},
	})

	client := NewClient("server1", mt)
	tool := NewMCPTool(client, ToolInfo{Name: "greet"})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"world"}`))
	require.NoError(t, err)
	assert.Equal(t, "Hello from MCP!", result)
}

func TestMCPTool_Execute_MultiContent(t *testing.T) {
	mt := newMockTransport()
	mt.SetResponse("tools/call", ToolCallResult{
		Content: []ToolContent{
			{Type: "text", Text: "line 1"},
			{Type: "text", Text: "line 2"},
		},
	})

	client := NewClient("server1", mt)
	tool := NewMCPTool(client, ToolInfo{Name: "multi"})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "line 1\nline 2", result)
}

func TestMCPTool_Execute_Error(t *testing.T) {
	mt := newMockTransport()
	mt.SetResponse("tools/call", ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: "bad request"}},
		IsError: true,
	})

	client := NewClient("server1", mt)
	tool := NewMCPTool(client, ToolInfo{Name: "bad"})

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.Equal(t, "bad request", result)
}

func TestMCPTool_Execute_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewClient("server1", newMockTransport())
	tool := NewMCPTool(client, ToolInfo{Name: "test"})

	_, err := tool.Execute(ctx, json.RawMessage(`{}`))
	assert.Error(t, err)
}

func TestManager_NewManager(t *testing.T) {
	m := NewManager()
	assert.NotNil(t, m.clients)
	assert.Empty(t, m.Clients())
}

func TestManager_Client_NotFound(t *testing.T) {
	m := NewManager()
	assert.Nil(t, m.Client("nonexistent"))
}

func TestManager_Close_Empty(t *testing.T) {
	m := NewManager()
	m.Close() // should not panic
	assert.Empty(t, m.Clients())
}

func TestServerConfig_Fields(t *testing.T) {
	cfg := ServerConfig{
		Name:    "test-server",
		Command: "/usr/bin/node",
		Args:    []string{"server.js"},
		Env:     []string{"PORT=3000"},
	}
	assert.Equal(t, "test-server", cfg.Name)
	assert.Equal(t, "/usr/bin/node", cfg.Command)
}
