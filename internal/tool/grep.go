package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/xmh1011/glaude/internal/telemetry"
)

const grepMaxResults = 250

// GrepTool searches file contents using ripgrep (rg) or grep.
// Results are capped at 250 matches. Always excludes .git, node_modules, etc.
type GrepTool struct{}

type grepInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	Glob       string `json:"glob"`
	OutputMode string `json:"output_mode"`
	Context    int    `json:"context"`
	CaseInsens bool   `json:"-i"`
}

func (g *GrepTool) Name() string { return "Grep" }

func (g *GrepTool) Description() string {
	return "Search tool built on ripgrep. Supports regex patterns, file type filtering via glob parameter, and multiple output modes (content, files_with_matches, count)."
}

func (g *GrepTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "The regex pattern to search for"},
			"path": {"type": "string", "description": "File or directory to search in. Defaults to current working directory."},
			"glob": {"type": "string", "description": "Glob pattern to filter files (e.g. \"*.go\", \"*.{ts,tsx}\")"},
			"output_mode": {"type": "string", "enum": ["content", "files_with_matches", "count"], "description": "Output mode. Defaults to files_with_matches."},
			"context": {"type": "integer", "description": "Number of context lines before and after each match (for content mode)"},
			"-i": {"type": "boolean", "description": "Case insensitive search"}
		},
		"required": ["pattern"]
	}`)
}

func (g *GrepTool) IsReadOnly() bool { return true }

func (g *GrepTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in grepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	if in.OutputMode == "" {
		in.OutputMode = "files_with_matches"
	}

	searchPath := in.Path
	if searchPath == "" {
		searchPath = "."
	}

	// Prefer rg, fall back to grep
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return g.execGrep(ctx, in, searchPath)
	}
	return g.execRg(ctx, rgPath, in, searchPath)
}

func (g *GrepTool) execRg(ctx context.Context, rgPath string, in grepInput, searchPath string) (string, error) {
	args := []string{
		"--no-heading",
		"--line-number",
		"--color", "never",
		"--max-count", strconv.Itoa(grepMaxResults),
	}

	// Exclude default directories
	for _, ex := range defaultExcludes {
		args = append(args, "--glob", "!"+ex)
	}

	// Output mode
	switch in.OutputMode {
	case "files_with_matches":
		args = append(args, "--files-with-matches")
	case "count":
		args = append(args, "--count")
	case "content":
		// default rg output with line numbers
	}

	// Context lines
	if in.Context > 0 && in.OutputMode == "content" {
		args = append(args, "-C", strconv.Itoa(in.Context))
	}

	// Case insensitive
	if in.CaseInsens {
		args = append(args, "-i")
	}

	// File glob filter
	if in.Glob != "" {
		args = append(args, "--glob", in.Glob)
	}

	args = append(args, in.Pattern, searchPath)

	cmd := exec.CommandContext(ctx, rgPath, args...)
	out, err := cmd.Output()
	result := string(out)

	if err != nil {
		// rg exits with 1 if no matches found — that's not an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "No matches found.", nil
		}
		// Exit code 2 means error
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("rg error (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("running rg: %w", err)
	}

	result = truncateLines(result, grepMaxResults)

	telemetry.Log.
		WithField("pattern", in.Pattern).
		WithField("mode", in.OutputMode).
		Debug("grep: search complete")

	return result, nil
}

func (g *GrepTool) execGrep(ctx context.Context, in grepInput, searchPath string) (string, error) {
	args := []string{
		"-r", "-n",
		"--color=never",
	}

	// Exclude default directories
	for _, ex := range defaultExcludes {
		args = append(args, "--exclude-dir="+ex)
	}

	// Output mode
	switch in.OutputMode {
	case "files_with_matches":
		args = append(args, "-l")
	case "count":
		args = append(args, "-c")
	}

	// Context lines
	if in.Context > 0 && in.OutputMode == "content" {
		args = append(args, "-C", strconv.Itoa(in.Context))
	}

	// Case insensitive
	if in.CaseInsens {
		args = append(args, "-i")
	}

	// File glob filter
	if in.Glob != "" {
		args = append(args, "--include="+in.Glob)
	}

	args = append(args, in.Pattern, searchPath)

	cmd := exec.CommandContext(ctx, "grep", args...)
	out, err := cmd.Output()
	result := string(out)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "No matches found.", nil
		}
		return "", fmt.Errorf("running grep: %w", err)
	}

	result = truncateLines(result, grepMaxResults)
	return result, nil
}

// truncateLines limits output to maxLines.
func truncateLines(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, fmt.Sprintf("\n(results truncated to %d lines)", maxLines))
	}
	return strings.Join(lines, "\n")
}
