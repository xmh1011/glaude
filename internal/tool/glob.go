package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"glaude/internal/telemetry"
)

const globMaxResults = 100

// defaultExcludes are directories always excluded from glob/grep results.
var defaultExcludes = []string{
	".git",
	"node_modules",
	"vendor",
	"__pycache__",
	".DS_Store",
}

// GlobTool finds files matching a glob pattern.
// Results are sorted by modification time (most recent first) and
// capped at 100 entries. Always excludes .git, node_modules, etc.
type GlobTool struct{}

type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func (g *GlobTool) Name() string { return "Glob" }

func (g *GlobTool) Description() string {
	return "Fast file pattern matching tool. Supports glob patterns like \"**/*.go\" or \"src/**/*.ts\". Returns matching file paths sorted by modification time (most recent first)."
}

func (g *GlobTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "The glob pattern to match files against"},
			"path": {"type": "string", "description": "The directory to search in. Defaults to current working directory."}
		},
		"required": ["pattern"]
	}`)
}

func (g *GlobTool) IsReadOnly() bool { return true }

func (g *GlobTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in globInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	root := in.Path
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getting cwd: %w", err)
		}
	}

	type fileEntry struct {
		path    string
		modTime int64
	}

	var matches []fileEntry

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		// Check cancellation periodically during walk
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Exclude default directories
		if info.IsDir() {
			base := info.Name()
			for _, ex := range defaultExcludes {
				if base == ex {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Get relative path for matching
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		// Match against pattern
		matched, err := filepath.Match(in.Pattern, filepath.Base(path))
		if err != nil {
			return nil
		}
		// Also try matching the full relative path for patterns like "src/**/*.go"
		if !matched {
			matched, _ = filepath.Match(in.Pattern, relPath)
		}
		// For ** patterns, do a more flexible match
		if !matched && strings.Contains(in.Pattern, "**") {
			matched = doubleStarMatch(in.Pattern, relPath)
		}

		if matched {
			matches = append(matches, fileEntry{path: path, modTime: info.ModTime().Unix()})
		}

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walking %s: %w", root, err)
	}

	// Sort by modification time descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime > matches[j].modTime
	})

	// Cap results
	truncated := false
	if len(matches) > globMaxResults {
		matches = matches[:globMaxResults]
		truncated = true
	}

	if len(matches) == 0 {
		return "No files matched the pattern.", nil
	}

	var b strings.Builder
	for _, m := range matches {
		fmt.Fprintln(&b, m.path)
	}
	if truncated {
		fmt.Fprintf(&b, "\n(results truncated to %d entries)\n", globMaxResults)
	}

	telemetry.Log.
		WithField("pattern", in.Pattern).
		WithField("matches", len(matches)).
		Debug("glob: search complete")

	return b.String(), nil
}

// doubleStarMatch handles ** glob patterns by splitting on ** and matching
// each segment against path components.
func doubleStarMatch(pattern, path string) bool {
	// Split pattern on "**/"
	parts := strings.Split(pattern, "**/")
	if len(parts) < 2 {
		// Try "**" without trailing slash
		parts = strings.Split(pattern, "**")
		if len(parts) < 2 {
			return false
		}
	}

	// The suffix after ** is what matters (e.g., "*.go")
	suffix := parts[len(parts)-1]
	if suffix == "" {
		return true // pattern ends with **, matches everything
	}

	// Check if the filename matches the suffix pattern
	matched, _ := filepath.Match(suffix, filepath.Base(path))
	if matched {
		// If there's a prefix, check it too
		prefix := parts[0]
		if prefix == "" {
			return true
		}
		return strings.HasPrefix(path, prefix)
	}
	return false
}
