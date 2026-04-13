package taskcreate

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
	assert.Equal(t, "TaskCreate", tool.Name())
	assert.False(t, tool.IsReadOnly())
	assert.NotEmpty(t, tool.Description())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
	assert.Equal(t, "object", schema["type"])
}

func TestTool_Execute(t *testing.T) {
	t.Run("create task", func(t *testing.T) {
		st := state.New()
		tool := &Tool{State: st}

		input, _ := json.Marshal(Input{
			Subject:     "Fix bug",
			Description: "Fix the login bug",
			ActiveForm:  "Fixing bug",
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Task #1 created")
		assert.Contains(t, result, "Fix bug")

		task := st.GetTask("1")
		require.NotNil(t, task)
		assert.Equal(t, "Fix bug", task.Subject)
		assert.Equal(t, "Fixing bug", task.ActiveForm)
	})

	t.Run("increments ID", func(t *testing.T) {
		st := state.New()
		tool := &Tool{State: st}

		input1, _ := json.Marshal(Input{Subject: "A", Description: "A"})
		input2, _ := json.Marshal(Input{Subject: "B", Description: "B"})
		tool.Execute(context.Background(), input1)
		result, err := tool.Execute(context.Background(), input2)
		require.NoError(t, err)
		assert.Contains(t, result, "Task #2")
	})

	t.Run("missing subject", func(t *testing.T) {
		tool := &Tool{State: state.New()}
		input, _ := json.Marshal(Input{Description: "desc"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})

	t.Run("missing description", func(t *testing.T) {
		tool := &Tool{State: state.New()}
		input, _ := json.Marshal(Input{Subject: "sub"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})

	t.Run("with metadata", func(t *testing.T) {
		st := state.New()
		tool := &Tool{State: st}

		input, _ := json.Marshal(Input{
			Subject:     "Meta task",
			Description: "Task with metadata",
			Metadata:    map[string]any{"priority": "high"},
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Meta task")

		task := st.GetTask("1")
		assert.Equal(t, "high", task.Metadata["priority"])
	})
}
