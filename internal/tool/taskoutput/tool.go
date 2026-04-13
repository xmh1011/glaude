// Package taskoutput implements the TaskOutput tool for retrieving task output.
package taskoutput

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool retrieves output from a running or completed background task. Currently a stub.
type Tool struct{}

// Input is the parsed input for the TaskOutput tool.
type Input struct {
	TaskID  string `json:"task_id"`
	Block   *bool  `json:"block,omitempty"`
	Timeout *int   `json:"timeout,omitempty"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "TaskOutput" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Retrieves output from a running or completed background task. Use block=true to wait for completion."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {"type": "string", "description": "The task ID to get output from"},
			"block": {"type": "boolean", "description": "Whether to wait for completion", "default": true},
			"timeout": {"type": "integer", "description": "Max wait time in ms", "default": 30000, "maximum": 600000}
		},
		"required": ["task_id"]
	}`)
}

// IsReadOnly returns true since this only reads output.
func (t *Tool) IsReadOnly() bool { return true }

// Execute retrieves task output. Currently returns stub response.
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

	return fmt.Sprintf("Background task output retrieval is not supported yet (task_id: %s).", in.TaskID), nil
}
