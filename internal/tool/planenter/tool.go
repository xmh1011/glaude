// Package planenter implements the EnterPlanMode tool.
package planenter

import (
	"context"
	"encoding/json"

	"github.com/xmh1011/glaude/internal/permission"
	"github.com/xmh1011/glaude/internal/state"
)

// Tool enters plan mode, restricting mutations to read-only operations.
type Tool struct {
	State   *state.State
	Checker *permission.Checker
}

// Name returns the tool name.
func (t *Tool) Name() string { return "EnterPlanMode" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Enters plan mode where only read-only tools are allowed. Use this to explore the codebase and design an implementation approach before writing code."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`)
}

// IsReadOnly returns true since entering plan mode is not itself a mutation.
func (t *Tool) IsReadOnly() bool { return true }

// Execute enters plan mode and saves the current permission mode.
func (t *Tool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if t.State.InPlanMode() {
		return "Already in plan mode.", nil
	}

	// Save current mode and switch to plan-only.
	prevMode := t.Checker.Mode().String()
	t.State.SetPlanMode(true, prevMode)
	t.Checker.SetMode(permission.ModePlanOnly)

	return "Entered plan mode. Only read-only tools are allowed. Use ExitPlanMode when your plan is ready for review.", nil
}
