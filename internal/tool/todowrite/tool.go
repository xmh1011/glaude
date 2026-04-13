// Package todowrite implements the TodoWrite tool for managing a todo list.
package todowrite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xmh1011/glaude/internal/state"
)

// Tool atomically replaces the session todo list.
type Tool struct {
	State *state.State
}

// Input is the parsed input for the TodoWrite tool.
type Input struct {
	Todos []TodoEntry `json:"todos"`
}

// TodoEntry is a single todo item in the input.
type TodoEntry struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm,omitempty"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "TodoWrite" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return `Atomically replaces the session todo list. Use this to track progress on multi-step tasks. When all items are marked "completed", the list is automatically cleared.`
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"todos": {
				"type": "array",
				"description": "The complete todo list to set.",
				"items": {
					"type": "object",
					"properties": {
						"content": {"type": "string", "description": "The todo item text"},
						"status": {"type": "string", "enum": ["pending", "in_progress", "completed"], "description": "Current status"},
						"activeForm": {"type": "string", "description": "Present continuous form shown when in_progress"}
					},
					"required": ["content", "status"]
				}
			}
		},
		"required": ["todos"]
	}`)
}

// IsReadOnly returns false since this modifies session state.
func (t *Tool) IsReadOnly() bool { return false }

// Execute replaces the todo list and returns a summary.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	items := make([]state.TodoItem, len(in.Todos))
	for i, entry := range in.Todos {
		items[i] = state.TodoItem{
			Content:    entry.Content,
			Status:     state.TodoStatus(entry.Status),
			ActiveForm: entry.ActiveForm,
		}
	}

	t.State.SetTodos(items)

	// Return summary.
	current := t.State.Todos()
	if len(current) == 0 {
		return "Todo list cleared (all items completed).", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Todo list updated (%d items):\n", len(current))
	for i, item := range current {
		marker := "[ ]"
		switch item.Status {
		case state.TodoInProgress:
			marker = "[~]"
		case state.TodoCompleted:
			marker = "[x]"
		}
		fmt.Fprintf(&b, "%d. %s %s\n", i+1, marker, item.Content)
	}
	return b.String(), nil
}
