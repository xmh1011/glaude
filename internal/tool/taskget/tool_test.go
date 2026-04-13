package taskget

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
	assert.Equal(t, "TaskGet", tool.Name())
	assert.True(t, tool.IsReadOnly())
	assert.NotEmpty(t, tool.Description())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
	assert.Equal(t, "object", schema["type"])
}

func TestTool_Execute(t *testing.T) {
	t.Run("get existing task", func(t *testing.T) {
		st := state.New()
		id := st.CreateTask("Bug fix", "Fix login bug", "Fixing", nil)
		tool := &Tool{State: st}

		input, _ := json.Marshal(Input{TaskID: id})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Bug fix")
		assert.Contains(t, result, "pending")
		assert.Contains(t, result, "Fix login bug")
	})

	t.Run("task not found", func(t *testing.T) {
		tool := &Tool{State: state.New()}
		input, _ := json.Marshal(Input{TaskID: "999"})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "not found")
	})

	t.Run("missing taskId", func(t *testing.T) {
		tool := &Tool{State: state.New()}
		input, _ := json.Marshal(Input{})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})

	t.Run("with dependencies", func(t *testing.T) {
		st := state.New()
		id1 := st.CreateTask("Task 1", "First", "", nil)
		id2 := st.CreateTask("Task 2", "Second", "", nil)
		st.UpdateTask(id2, state.UpdateOpts{AddBlockedBy: []string{id1}})

		tool := &Tool{State: st}
		input, _ := json.Marshal(Input{TaskID: id2})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Blocked by")
	})
}
