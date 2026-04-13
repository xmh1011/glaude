// Package tasklist implements the TaskList tool for listing all tasks.
package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xmh1011/glaude/internal/state"
)

// Tool lists all non-deleted tasks in the session.
type Tool struct {
	State *state.State
}

// Name returns the tool name.
func (t *Tool) Name() string { return "TaskList" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Lists all tasks in the session with their ID, subject, status, owner, and blocking dependencies."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`)
}

// IsReadOnly returns true since this only reads state.
func (t *Tool) IsReadOnly() bool { return true }

// Execute lists all tasks, filtering completed tasks from blockedBy.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	tasks := t.State.AllTasks()
	if len(tasks) == 0 {
		return "No tasks.", nil
	}

	// Build a set of completed task IDs for filtering blockedBy.
	completedSet := make(map[string]bool)
	for _, task := range tasks {
		if task.Status == state.TaskCompleted {
			completedSet[task.ID] = true
		}
	}

	var b strings.Builder
	for _, task := range tasks {
		// Filter completed tasks from blockedBy display.
		var openBlockers []string
		for bid := range task.BlockedBy {
			if !completedSet[bid] {
				openBlockers = append(openBlockers, bid)
			}
		}

		statusIcon := statusChar(task.Status)
		line := fmt.Sprintf("#%s. [%s] %s", task.ID, statusIcon, task.Subject)
		if task.Owner != "" {
			line += fmt.Sprintf(" (owner: %s)", task.Owner)
		}
		if len(openBlockers) > 0 {
			line += fmt.Sprintf(" [blocked by: %s]", strings.Join(openBlockers, ", "))
		}
		b.WriteString(line + "\n")
	}

	return b.String(), nil
}

func statusChar(s state.TaskStatus) string {
	switch s {
	case state.TaskPending:
		return "pending"
	case state.TaskInProgress:
		return "in_progress"
	case state.TaskCompleted:
		return "completed"
	default:
		return string(s)
	}
}
