package mcpreslist

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/mcp"
)

func TestTool_Metadata(t *testing.T) {
	tool := &Tool{Manager: mcp.NewManager()}
	assert.Equal(t, "ListMcpResources", tool.Name())
	assert.True(t, tool.IsReadOnly())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
}

func TestTool_Execute(t *testing.T) {
	t.Run("no servers", func(t *testing.T) {
		tool := &Tool{Manager: mcp.NewManager()}
		result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
		require.NoError(t, err)
		assert.Contains(t, result, "No MCP servers")
	})

	t.Run("server filter not found", func(t *testing.T) {
		// Manager with no clients means filter won't find anything
		tool := &Tool{Manager: mcp.NewManager()}
		input, _ := json.Marshal(Input{Server: "nonexistent"})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "No MCP servers")
	})
}
