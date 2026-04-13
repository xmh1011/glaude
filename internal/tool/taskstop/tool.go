// Package taskstop implements the TaskStop tool for stopping background tasks.
package taskstop

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool stops a running background task. Currently a stub.
type Tool struct{}

// Input is the parsed input for the TaskStop tool.
type Input struct {
	TaskID string `json:"task_id"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "TaskStop" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Stops a running background task by its ID. Returns success or failure status."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {"type": "string", "description": "The ID of the background task to stop"}
		},
		"required": ["task_id"]
	}`)
}

// IsReadOnly returns false since stopping a task is a mutation.
func (t *Tool) IsReadOnly() bool { return false }

// Execute attempts to stop a background task. Currently returns stub response.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if in.TaskID == "" {
		return "", fmt.Errorf("task_id is required")
	}

	return fmt.Sprintf("Background task stopping is not supported yet (task_id: %s).", in.TaskID), nil
}
