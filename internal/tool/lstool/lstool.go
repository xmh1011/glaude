package lstool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LSTool lists directory contents (non-recursive).
type LSTool struct{}

// Input is the parsed input for the LS tool.
type Input struct {
	Path string `json:"path"`
}

func (l *LSTool) Name() string { return "LS" }

func (l *LSTool) Description() string {
	return "Lists files and directories in a given path. Returns name, type (file/dir), and size for each entry."
}

func (l *LSTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "The directory path to list. Defaults to current working directory."}
		}
	}`)
}

func (l *LSTool) IsReadOnly() bool { return true }

func (l *LSTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	dir := in.Path
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getting cwd: %w", err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", dir, err)
	}

	if len(entries) == 0 {
		return fmt.Sprintf("%s (empty directory)", dir), nil
	}

	var b strings.Builder
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}

		name := e.Name()
		if e.IsDir() {
			name += "/"
			fmt.Fprintf(&b, "%-40s  <dir>\n", name)
		} else {
			fmt.Fprintf(&b, "%-40s  %s\n", name, formatSize(info.Size()))
		}
	}

	absDir, _ := filepath.Abs(dir)
	return fmt.Sprintf("%s\n%s", absDir, b.String()), nil
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
