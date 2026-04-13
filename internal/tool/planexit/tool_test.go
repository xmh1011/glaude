package planexit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/permission"
	"github.com/xmh1011/glaude/internal/state"
)

func TestTool_Metadata(t *testing.T) {
	tool := &Tool{State: state.New(), Checker: permission.NewCheckerWithMode(permission.ModeDefault)}
	assert.Equal(t, "ExitPlanMode", tool.Name())
	assert.False(t, tool.IsReadOnly())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
}

func TestTool_Execute(t *testing.T) {
	t.Run("exit plan mode", func(t *testing.T) {
		st := state.New()
		checker := permission.NewCheckerWithMode(permission.ModePlanOnly)
		st.SetPlanMode(true, "auto-edit")
		tool := &Tool{State: st, Checker: checker}

		result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
		require.NoError(t, err)
		assert.Contains(t, result, "Exited plan mode")
		assert.Contains(t, result, "auto-edit")
		assert.False(t, st.InPlanMode())
		assert.Equal(t, permission.ModeAutoEdit, checker.Mode())
	})

	t.Run("not in plan mode", func(t *testing.T) {
		st := state.New()
		checker := permission.NewCheckerWithMode(permission.ModeDefault)
		tool := &Tool{State: st, Checker: checker}

		result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
		require.NoError(t, err)
		assert.Contains(t, result, "Not in plan mode")
	})
}
