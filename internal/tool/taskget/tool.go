// Package taskget implements the TaskGet tool for retrieving task details.
package taskget

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/xmh1011/glaude/internal/state"
)

// Tool retrieves detailed information about a specific task.
type Tool struct {
	State *state.State
}

// Input is the parsed input for the TaskGet tool.
type Input struct {
	TaskID string `json:"taskId"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "TaskGet" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Retrieves full details of a task by its ID, including subject, description, status, owner, and dependencies."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"taskId": {"type": "string", "description": "The ID of the task to retrieve"}
		},
		"required": ["taskId"]
	}`)
}

// IsReadOnly returns true since this only reads state.
func (t *Tool) IsReadOnly() bool { return true }

// Execute retrieves and formats a task's details.
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

	task := t.State.GetTask(in.TaskID)
	if task == nil {
		return fmt.Sprintf("Task %q not found.", in.TaskID), nil
	}

	return formatTask(task), nil
}

func formatTask(task *state.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task #%s: %s\n", task.ID, task.Subject)
	fmt.Fprintf(&b, "Status: %s\n", task.Status)
	if task.Owner != "" {
		fmt.Fprintf(&b, "Owner: %s\n", task.Owner)
	}
	if task.ActiveForm != "" {
		fmt.Fprintf(&b, "Active Form: %s\n", task.ActiveForm)
	}
	if task.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", task.Description)
	}
	if len(task.Blocks) > 0 {
		fmt.Fprintf(&b, "Blocks: %s\n", sortedKeys(task.Blocks))
	}
	if len(task.BlockedBy) > 0 {
		fmt.Fprintf(&b, "Blocked by: %s\n", sortedKeys(task.BlockedBy))
	}
	if len(task.Metadata) > 0 {
		fmt.Fprintf(&b, "Metadata: %v\n", task.Metadata)
	}
	return b.String()
}

func sortedKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
