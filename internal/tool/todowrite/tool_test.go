package todowrite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/state"
)

func TestTool_Metadata(t *testing.T) {
	tool := &Tool{State: state.New()}
	assert.Equal(t, "TodoWrite", tool.Name())
	assert.False(t, tool.IsReadOnly())
	assert.NotEmpty(t, tool.Description())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
	assert.Equal(t, "object", schema["type"])
}

func TestTool_Execute(t *testing.T) {
	t.Run("set items", func(t *testing.T) {
		st := state.New()
		tool := &Tool{State: st}

		input, _ := json.Marshal(Input{
			Todos: []TodoEntry{
				{Content: "Task A", Status: "pending"},
				{Content: "Task B", Status: "in_progress"},
			},
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "2 items")
		assert.Contains(t, result, "Task A")
		assert.Contains(t, result, "Task B")

		todos := st.Todos()
		require.Len(t, todos, 2)
	})

	t.Run("all completed clears", func(t *testing.T) {
		st := state.New()
		tool := &Tool{State: st}

		input, _ := json.Marshal(Input{
			Todos: []TodoEntry{
				{Content: "Done", Status: "completed"},
			},
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "cleared")
		assert.Empty(t, st.Todos())
	})

	t.Run("invalid JSON", func(t *testing.T) {
		tool := &Tool{State: state.New()}
		_, err := tool.Execute(context.Background(), json.RawMessage(`{bad`))
		assert.Error(t, err)
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		tool := &Tool{State: state.New()}
		_, err := tool.Execute(ctx, json.RawMessage(`{}`))
		assert.Error(t, err)
	})
}
