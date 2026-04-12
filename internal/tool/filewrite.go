package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileWriteTool creates new files or completely overwrites existing ones.
// Parent directories are created automatically.
type FileWriteTool struct{}

type fileWriteInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func (f *FileWriteTool) Name() string { return "Write" }

func (f *FileWriteTool) Description() string {
	return "Writes content to a file, creating it if it doesn't exist or overwriting if it does. Parent directories are created automatically."
}

func (f *FileWriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {"type": "string", "description": "The absolute path to the file to write"},
			"content": {"type": "string", "description": "The content to write to the file"}
		},
		"required": ["file_path", "content"]
	}`)
}

func (f *FileWriteTool) IsReadOnly() bool { return false }

func (f *FileWriteTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in fileWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}

	// Create parent directories
	dir := filepath.Dir(in.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating directories %s: %w", dir, err)
	}

	if err := os.WriteFile(in.FilePath, []byte(in.Content), 0644); err != nil {
		return "", fmt.Errorf("writing %s: %w", in.FilePath, err)
	}

	return fmt.Sprintf("Successfully wrote to %s (%d bytes)", in.FilePath, len(in.Content)), nil
}
