package mcpresread

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
	assert.Equal(t, "ReadMcpResource", tool.Name())
	assert.True(t, tool.IsReadOnly())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
}

func TestTool_Execute(t *testing.T) {
	t.Run("server not found", func(t *testing.T) {
		tool := &Tool{Manager: mcp.NewManager()}
		input, _ := json.Marshal(Input{Server: "nonexistent", URI: "file:///test"})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "not found")
	})

	t.Run("missing server", func(t *testing.T) {
		tool := &Tool{Manager: mcp.NewManager()}
		input, _ := json.Marshal(Input{URI: "file:///test"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})

	t.Run("missing URI", func(t *testing.T) {
		tool := &Tool{Manager: mcp.NewManager()}
		input, _ := json.Marshal(Input{Server: "test"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})
}
