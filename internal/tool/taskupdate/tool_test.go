package taskupdate

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
	assert.Equal(t, "TaskUpdate", tool.Name())
	assert.False(t, tool.IsReadOnly())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
}

func TestTool_Execute(t *testing.T) {
	t.Run("update status", func(t *testing.T) {
		st := state.New()
		id := st.CreateTask("Test", "desc", "", nil)
		tool := &Tool{State: st}

		status := "in_progress"
		input, _ := json.Marshal(Input{TaskID: id, Status: &status})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Updated")

		task := st.GetTask(id)
		assert.Equal(t, state.TaskInProgress, task.Status)
	})

	t.Run("delete task", func(t *testing.T) {
		st := state.New()
		id := st.CreateTask("Delete me", "desc", "", nil)
		tool := &Tool{State: st}

		status := "deleted"
		input, _ := json.Marshal(Input{TaskID: id, Status: &status})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "deleted")
	})

	t.Run("task not found", func(t *testing.T) {
		tool := &Tool{State: state.New()}
		input, _ := json.Marshal(Input{TaskID: "999"})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Error")
	})

	t.Run("missing taskId", func(t *testing.T) {
		tool := &Tool{State: state.New()}
		input, _ := json.Marshal(Input{})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})

	t.Run("add dependencies", func(t *testing.T) {
		st := state.New()
		id1 := st.CreateTask("A", "a", "", nil)
		id2 := st.CreateTask("B", "b", "", nil)
		tool := &Tool{State: st}

		input, _ := json.Marshal(Input{TaskID: id2, AddBlockedBy: []string{id1}})
		_, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)

		task := st.GetTask(id2)
		assert.True(t, task.BlockedBy[id1])
	})
}
