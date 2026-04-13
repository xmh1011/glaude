package sleep

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTool_Metadata(t *testing.T) {
	tool := &Tool{}
	assert.Equal(t, "Sleep", tool.Name())
	assert.True(t, tool.IsReadOnly())
	assert.NotEmpty(t, tool.Description())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
	assert.Equal(t, "object", schema["type"])
}

func TestTool_Execute(t *testing.T) {
	tool := &Tool{}

	t.Run("short sleep", func(t *testing.T) {
		input, _ := json.Marshal(Input{Duration: 1})
		start := time.Now()
		result, err := tool.Execute(context.Background(), input)
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.Contains(t, result, "1 seconds")
		assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond)
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		input, _ := json.Marshal(Input{Duration: 60})
		_, err := tool.Execute(ctx, input)
		assert.Error(t, err)
	})

	t.Run("already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		input, _ := json.Marshal(Input{Duration: 1})
		_, err := tool.Execute(ctx, input)
		assert.Error(t, err)
	})

	t.Run("invalid duration zero", func(t *testing.T) {
		input, _ := json.Marshal(Input{Duration: 0})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})

	t.Run("duration too large", func(t *testing.T) {
		input, _ := json.Marshal(Input{Duration: 500})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), json.RawMessage(`{bad`))
		assert.Error(t, err)
	})
}
