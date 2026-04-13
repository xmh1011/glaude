package taskstop

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTool_Metadata(t *testing.T) {
	tool := &Tool{}
	assert.Equal(t, "TaskStop", tool.Name())
	assert.False(t, tool.IsReadOnly())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
}

func TestTool_Execute(t *testing.T) {
	tool := &Tool{}

	t.Run("stub response", func(t *testing.T) {
		input, _ := json.Marshal(Input{TaskID: "42"})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "not supported yet")
	})

	t.Run("missing task_id", func(t *testing.T) {
		input, _ := json.Marshal(Input{})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})
}
