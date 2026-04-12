package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// FileReadTool reads file contents with optional offset and line limit.
type FileReadTool struct{}

type fileReadInput struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

func (f *FileReadTool) Name() string { return "Read" }

func (f *FileReadTool) Description() string {
	return "Reads a file from the local filesystem. Returns the file contents with line numbers."
}

func (f *FileReadTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {"type": "string", "description": "The absolute path to the file to read"},
			"offset": {"type": "integer", "description": "Line number to start reading from (1-based). Defaults to 1."},
			"limit": {"type": "integer", "description": "Maximum number of lines to read. Defaults to 2000."}
		},
		"required": ["file_path"]
	}`)
}

func (f *FileReadTool) IsReadOnly() bool { return true }

func (f *FileReadTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in fileReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}
	if in.Limit <= 0 {
		in.Limit = 2000
	}
	if in.Offset <= 0 {
		in.Offset = 1
	}

	data, err := os.ReadFile(in.FilePath)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", in.FilePath, err)
	}

	lines := strings.Split(string(data), "\n")
	// Remove trailing empty line from Split behavior
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	start := in.Offset - 1 // convert to 0-based
	if start >= len(lines) {
		return fmt.Sprintf("(file has %d lines, offset %d is past end)", len(lines), in.Offset), nil
	}
	end := start + in.Limit
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		line := lines[i]
		// Truncate very long lines
		if len(line) > 2000 {
			line = line[:2000] + "... (truncated)"
		}
		fmt.Fprintf(&b, "%6d\t%s\n", i+1, line)
	}
	return b.String(), nil
}
