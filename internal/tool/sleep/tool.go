// Package sleep implements the Sleep tool for waiting a specified duration.
package sleep

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const maxDuration = 300 // seconds

// Tool pauses execution for a specified number of seconds.
type Tool struct{}

// Input is the parsed input for the Sleep tool.
type Input struct {
	Duration int `json:"duration"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "Sleep" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Pauses execution for a specified number of seconds (max 300). Use this when you need to wait before checking on something."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"duration": {
				"type": "integer",
				"description": "Number of seconds to sleep (1-300)",
				"minimum": 1,
				"maximum": 300
			}
		},
		"required": ["duration"]
	}`)
}

// IsReadOnly returns true since sleeping doesn't modify state.
func (t *Tool) IsReadOnly() bool { return true }

// Execute sleeps for the specified duration, respecting context cancellation.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if in.Duration < 1 {
		return "", fmt.Errorf("duration must be at least 1 second")
	}
	if in.Duration > maxDuration {
		return "", fmt.Errorf("duration must be at most %d seconds", maxDuration)
	}

	timer := time.NewTimer(time.Duration(in.Duration) * time.Second)
	defer timer.Stop()

	select {
	case <-timer.C:
		return fmt.Sprintf("Slept for %d seconds.", in.Duration), nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
