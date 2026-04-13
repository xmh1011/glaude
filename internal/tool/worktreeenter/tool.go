// Package worktreeenter implements the EnterWorktree tool.
package worktreeenter

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/xmh1011/glaude/internal/state"
)

// Tool creates a git worktree and switches the session into it.
type Tool struct {
	State *state.State
}

// Input is the parsed input for the EnterWorktree tool.
type Input struct {
	Name string `json:"name,omitempty"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "EnterWorktree" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Creates an isolated git worktree and switches the session into it. Use this for parallel work without affecting the main branch."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "Optional name for the worktree. A random name is generated if not provided."}
		}
	}`)
}

// IsReadOnly returns false since this creates a worktree and changes directories.
func (t *Tool) IsReadOnly() bool { return false }

// Execute creates a worktree and switches into it.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if t.State.Worktree() != nil {
		return "Already in a worktree session.", nil
	}

	// Generate name if not provided.
	name := in.Name
	if name == "" {
		name = randomName()
	}

	// Sanitize name for branch/path usage.
	name = sanitize(name)

	origWD, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}

	worktreeDir := filepath.Join(origWD, ".glaude", "worktrees", name)
	branch := "glaude-" + name

	// Create the worktree.
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-b", branch, worktreeDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Switch into the worktree.
	if err := os.Chdir(worktreeDir); err != nil {
		return "", fmt.Errorf("chdir to worktree: %w", err)
	}

	t.State.SetWorktree(&state.WorktreeSession{
		Path:       worktreeDir,
		Branch:     branch,
		OriginalWD: origWD,
	})

	return fmt.Sprintf("Created worktree at %s (branch: %s). Session is now in the worktree.", worktreeDir, branch), nil
}

func sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)
	return strings.Trim(s, "-")
}

func randomName() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[r.Intn(len(charset))]
	}
	return string(b)
}
