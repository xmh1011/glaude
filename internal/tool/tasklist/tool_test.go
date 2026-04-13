package tasklist

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
	assert.Equal(t, "TaskList", tool.Name())
	assert.True(t, tool.IsReadOnly())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
}

func TestTool_Execute(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		tool := &Tool{State: state.New()}
		result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
		require.NoError(t, err)
		assert.Equal(t, "No tasks.", result)
	})

	t.Run("list tasks", func(t *testing.T) {
		st := state.New()
		st.CreateTask("Task A", "desc A", "", nil)
		st.CreateTask("Task B", "desc B", "", nil)

		tool := &Tool{State: st}
		result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
		require.NoError(t, err)
		assert.Contains(t, result, "Task A")
		assert.Contains(t, result, "Task B")
		assert.Contains(t, result, "#1")
		assert.Contains(t, result, "#2")
	})

	t.Run("filters completed from blockedBy", func(t *testing.T) {
		st := state.New()
		id1 := st.CreateTask("Done", "done", "", nil)
		id2 := st.CreateTask("Blocked", "blocked", "", nil)
		st.UpdateTask(id1, state.UpdateOpts{Status: state.Ptr(state.TaskCompleted)})
		st.UpdateTask(id2, state.UpdateOpts{AddBlockedBy: []string{id1}})

		tool := &Tool{State: st}
		result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
		require.NoError(t, err)
		// Completed task should not show in blockedBy
		assert.NotContains(t, result, "blocked by")
	})

	t.Run("shows owner", func(t *testing.T) {
		st := state.New()
		id := st.CreateTask("Owned", "owned", "", nil)
		st.UpdateTask(id, state.UpdateOpts{Owner: state.Ptr("agent-1")})

		tool := &Tool{State: st}
		result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
		require.NoError(t, err)
		assert.Contains(t, result, "agent-1")
	})
}
