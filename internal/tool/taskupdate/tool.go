// Package taskupdate implements the TaskUpdate tool for modifying tasks.
package taskupdate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xmh1011/glaude/internal/state"
)

// Tool updates an existing task's fields.
type Tool struct {
	State *state.State
}

// Input is the parsed input for the TaskUpdate tool.
type Input struct {
	TaskID      string         `json:"taskId"`
	Subject     *string        `json:"subject,omitempty"`
	Description *string        `json:"description,omitempty"`
	ActiveForm  *string        `json:"activeForm,omitempty"`
	Status      *string        `json:"status,omitempty"`
	Owner       *string        `json:"owner,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	AddBlocks   []string       `json:"addBlocks,omitempty"`
	AddBlockedBy []string      `json:"addBlockedBy,omitempty"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "TaskUpdate" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return `Updates a task's fields. Supports status (pending/in_progress/completed/deleted), subject, description, owner, metadata merge, and dependency management via addBlocks/addBlockedBy.`
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"taskId": {"type": "string", "description": "The ID of the task to update"},
			"subject": {"type": "string", "description": "New subject for the task"},
			"description": {"type": "string", "description": "New description for the task"},
			"activeForm": {"type": "string", "description": "Present continuous form shown in spinner when in_progress"},
			"status": {"type": "string", "enum": ["pending", "in_progress", "completed", "deleted"], "description": "New status for the task"},
			"owner": {"type": "string", "description": "New owner for the task"},
			"metadata": {"type": "object", "description": "Metadata keys to merge. Set a key to null to delete it."},
			"addBlocks": {"type": "array", "items": {"type": "string"}, "description": "Task IDs that this task blocks"},
			"addBlockedBy": {"type": "array", "items": {"type": "string"}, "description": "Task IDs that block this task"}
		},
		"required": ["taskId"]
	}`)
}

// IsReadOnly returns false since this modifies session state.
func (t *Tool) IsReadOnly() bool { return false }

// Execute updates the task and returns a confirmation.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if in.TaskID == "" {
		return "", fmt.Errorf("taskId is required")
	}

	opts := state.UpdateOpts{
		Subject:      in.Subject,
		Description:  in.Description,
		ActiveForm:   in.ActiveForm,
		Owner:        in.Owner,
		Metadata:     in.Metadata,
		AddBlocks:    in.AddBlocks,
		AddBlockedBy: in.AddBlockedBy,
	}

	if in.Status != nil {
		s := state.TaskStatus(*in.Status)
		opts.Status = &s
	}

	if err := t.State.UpdateTask(in.TaskID, opts); err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}

	if in.Status != nil && *in.Status == "deleted" {
		return fmt.Sprintf("Task #%s deleted.", in.TaskID), nil
	}
	return fmt.Sprintf("Updated task #%s status", in.TaskID), nil
}
