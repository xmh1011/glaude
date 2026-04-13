// Package planexit implements the ExitPlanMode tool.
package planexit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xmh1011/glaude/internal/permission"
	"github.com/xmh1011/glaude/internal/state"
)

// Tool exits plan mode and restores the previous permission mode.
type Tool struct {
	State   *state.State
	Checker *permission.Checker
}

// Name returns the tool name.
func (t *Tool) Name() string { return "ExitPlanMode" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Exits plan mode and restores the previous permission mode. Use this when your plan is ready for user review."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`)
}

// IsReadOnly returns false since exiting plan mode changes permissions.
func (t *Tool) IsReadOnly() bool { return false }

// Execute exits plan mode and restores the previous permission mode.
func (t *Tool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if !t.State.InPlanMode() {
		return "Not in plan mode.", nil
	}

	prevMode := t.State.PrePlanMode()
	t.State.SetPlanMode(false, "")
	t.Checker.SetMode(permission.ParseMode(prevMode))

	return fmt.Sprintf("Exited plan mode. Permission mode restored to %q.", prevMode), nil
}
