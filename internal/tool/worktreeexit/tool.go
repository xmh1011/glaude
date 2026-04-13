// Package worktreeexit implements the ExitWorktree tool.
package worktreeexit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/xmh1011/glaude/internal/state"
)

// Tool exits the current worktree session.
type Tool struct {
	State *state.State
}

// Input is the parsed input for the ExitWorktree tool.
type Input struct {
	Action         string `json:"action"` // "keep" or "remove"
	DiscardChanges bool   `json:"discard_changes,omitempty"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "ExitWorktree" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return `Exits the current worktree session. Action "keep" preserves the worktree; "remove" cleans it up. Set discard_changes to allow removing dirty worktrees.`
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["keep", "remove"], "description": "Whether to keep or remove the worktree"},
			"discard_changes": {"type": "boolean", "description": "Allow removing worktree with uncommitted changes", "default": false}
		},
		"required": ["action"]
	}`)
}

// IsReadOnly returns false since this removes worktrees.
func (t *Tool) IsReadOnly() bool { return false }

// Execute exits the worktree session.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	ws := t.State.Worktree()
	if ws == nil {
		return "Not in a worktree session.", nil
	}

	if in.Action != "keep" && in.Action != "remove" {
		return "", fmt.Errorf("action must be 'keep' or 'remove'")
	}

	// Return to original directory first.
	if err := os.Chdir(ws.OriginalWD); err != nil {
		return "", fmt.Errorf("chdir to original: %w", err)
	}

	if in.Action == "remove" {
		// Check for dirty working tree if not discarding.
		if !in.DiscardChanges {
			cmd := exec.CommandContext(ctx, "git", "-C", ws.Path, "status", "--porcelain")
			out, err := cmd.Output()
			if err == nil && len(strings.TrimSpace(string(out))) > 0 {
				// Go back to worktree since we can't remove it.
				os.Chdir(ws.Path) //nolint:errcheck
				return "Worktree has uncommitted changes. Set discard_changes=true to force removal.", nil
			}
		}

		args := []string{"worktree", "remove"}
		if in.DiscardChanges {
			args = append(args, "--force")
		}
		args = append(args, ws.Path)
		cmd := exec.CommandContext(ctx, "git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git worktree remove: %s: %w", strings.TrimSpace(string(out)), err)
		}

		// Delete the branch.
		exec.CommandContext(ctx, "git", "branch", "-D", ws.Branch).Run() //nolint:errcheck
	}

	t.State.SetWorktree(nil)

	if in.Action == "keep" {
		return fmt.Sprintf("Exited worktree (kept at %s, branch: %s). Session returned to %s.", ws.Path, ws.Branch, ws.OriginalWD), nil
	}
	return fmt.Sprintf("Worktree removed. Session returned to %s.", ws.OriginalWD), nil
}
