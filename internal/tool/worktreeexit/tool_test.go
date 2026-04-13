package worktreeexit

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
	assert.Equal(t, "ExitWorktree", tool.Name())
	assert.False(t, tool.IsReadOnly())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
}

func TestTool_Execute(t *testing.T) {
	t.Run("not in worktree", func(t *testing.T) {
		tool := &Tool{State: state.New()}
		input, _ := json.Marshal(Input{Action: "keep"})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Not in a worktree")
	})

	t.Run("invalid action", func(t *testing.T) {
		st := state.New()
		st.SetWorktree(&state.WorktreeSession{Path: "/tmp/wt", OriginalWD: "/tmp"})
		tool := &Tool{State: st}

		input, _ := json.Marshal(Input{Action: "invalid"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		tool := &Tool{State: state.New()}
		_, err := tool.Execute(ctx, json.RawMessage(`{"action":"keep"}`))
		assert.Error(t, err)
	})
}
