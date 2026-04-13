package toolsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/tool"
)

// mockTool is a minimal tool for testing.
type mockTool struct {
	name string
	desc string
}

func (m *mockTool) Name() string                                                       { return m.name }
func (m *mockTool) Description() string                                                { return m.desc }
func (m *mockTool) InputSchema() json.RawMessage                                       { return json.RawMessage(`{"type":"object"}`) }
func (m *mockTool) IsReadOnly() bool                                                   { return true }
func (m *mockTool) Execute(_ context.Context, _ json.RawMessage) (string, error)       { return "", nil }

func newTestRegistry() *tool.Registry {
	reg := tool.NewRegistry()
	reg.Register(&mockTool{name: "Bash", desc: "Execute shell commands"})
	reg.Register(&mockTool{name: "Read", desc: "Read files from disk"})
	reg.Register(&mockTool{name: "Edit", desc: "Edit files using string replacement"})
	return reg
}

func TestTool_Metadata(t *testing.T) {
	tl := &Tool{Registry: newTestRegistry()}
	assert.Equal(t, "ToolSearch", tl.Name())
	assert.True(t, tl.IsReadOnly())

	var schema map[string]any
	err := json.Unmarshal(tl.InputSchema(), &schema)
	require.NoError(t, err)
}

func TestTool_Execute(t *testing.T) {
	t.Run("keyword search", func(t *testing.T) {
		tl := &Tool{Registry: newTestRegistry()}
		input, _ := json.Marshal(Input{Query: "file"})
		result, err := tl.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Read")
		assert.Contains(t, result, "Edit")
	})

	t.Run("select exact", func(t *testing.T) {
		tl := &Tool{Registry: newTestRegistry()}
		input, _ := json.Marshal(Input{Query: "select:Bash"})
		result, err := tl.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Bash")
		assert.Contains(t, result, "shell commands")
	})

	t.Run("select not found", func(t *testing.T) {
		tl := &Tool{Registry: newTestRegistry()}
		input, _ := json.Marshal(Input{Query: "select:NonExistent"})
		result, err := tl.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "No tool named")
	})

	t.Run("no matches", func(t *testing.T) {
		tl := &Tool{Registry: newTestRegistry()}
		input, _ := json.Marshal(Input{Query: "zzzzz"})
		result, err := tl.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "No tools matching")
	})

	t.Run("max results", func(t *testing.T) {
		reg := tool.NewRegistry()
		for i := 0; i < 20; i++ {
			reg.Register(&mockTool{
				name: fmt.Sprintf("Tool%d", i),
				desc: "common keyword match",
			})
		}
		tl := &Tool{Registry: reg}
		max := 3
		input, _ := json.Marshal(Input{Query: "common", MaxResults: &max})
		result, err := tl.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "3 tool(s)")
	})
}
