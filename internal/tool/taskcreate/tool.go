// Package taskcreate implements the TaskCreate tool for creating new tasks.
package taskcreate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xmh1011/glaude/internal/state"
)

// Tool creates a new task in the session task board.
type Tool struct {
	State *state.State
}

// Input is the parsed input for the TaskCreate tool.
type Input struct {
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	ActiveForm  string         `json:"activeForm,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "TaskCreate" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Creates a new task with a subject, description, and optional metadata. Returns the assigned task ID."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"subject": {"type": "string", "description": "A brief title for the task"},
			"description": {"type": "string", "description": "A detailed description of what needs to be done"},
			"activeForm": {"type": "string", "description": "Present continuous form shown in spinner when in_progress"},
			"metadata": {"type": "object", "description": "Arbitrary metadata to attach to the task"}
		},
		"required": ["subject", "description"]
	}`)
}

// IsReadOnly returns false since this modifies session state.
func (t *Tool) IsReadOnly() bool { return false }

// Execute creates a new task and returns a confirmation.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if in.Subject == "" {
		return "", fmt.Errorf("subject is required")
	}
	if in.Description == "" {
		return "", fmt.Errorf("description is required")
	}

	id := t.State.CreateTask(in.Subject, in.Description, in.ActiveForm, in.Metadata)
	return fmt.Sprintf("Task #%s created successfully: %s", id, in.Subject), nil
}
