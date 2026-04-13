package worktreeenter

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
	assert.Equal(t, "EnterWorktree", tool.Name())
	assert.False(t, tool.IsReadOnly())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
}

func TestTool_Execute(t *testing.T) {
	t.Run("already in worktree", func(t *testing.T) {
		st := state.New()
		st.SetWorktree(&state.WorktreeSession{Path: "/tmp/wt"})
		tool := &Tool{State: st}

		result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
		require.NoError(t, err)
		assert.Contains(t, result, "Already")
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		tool := &Tool{State: state.New()}
		_, err := tool.Execute(ctx, json.RawMessage(`{}`))
		assert.Error(t, err)
	})
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-feature", "my-feature"},
		{"my feature!", "my-feature"},
		{"hello world", "hello-world"},
	}
	for _, tt := range tests {
		got := sanitize(tt.input)
		assert.Equal(t, tt.want, got)
	}
}

func TestRandomName(t *testing.T) {
	name := randomName()
	assert.Len(t, name, 8)
}
