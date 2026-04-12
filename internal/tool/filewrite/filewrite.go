package filewrite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xmh1011/glaude/internal/memory"
	"github.com/xmh1011/glaude/internal/tool"
)

// Tool creates new files or completely overwrites existing ones.
// Parent directories are created automatically.
type Tool struct {
	Checkpoint *memory.Checkpoint
	FileState  *tool.FileStateCache
}

// Input is the parsed input for the Write tool.
type Input struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func (f *Tool) Name() string { return "Write" }

func (f *Tool) Description() string {
	return "Writes content to a file, creating it if it doesn't exist or overwriting if it does. Parent directories are created automatically."
}

func (f *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {"type": "string", "description": "The absolute path to the file to write"},
			"content": {"type": "string", "description": "The content to write to the file"}
		},
		"required": ["file_path", "content"]
	}`)
}

func (f *Tool) IsReadOnly() bool { return false }

func (f *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
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

	// Staleness check for existing files: verify the file was read first
	if f.FileState != nil {
		if _, statErr := os.Stat(in.FilePath); statErr == nil {
			// File exists — check staleness before overwriting
			if err := f.FileState.CheckStaleness(in.FilePath); err != nil {
				return "", err
			}
		}
	}

	// Checkpoint: save file state before mutation
	if f.Checkpoint != nil {
		txID := f.Checkpoint.NextTxID()
		if err := f.Checkpoint.Save(txID, in.FilePath); err != nil {
			return "", fmt.Errorf("checkpoint: %w", err)
		}
	}

	if err := os.WriteFile(in.FilePath, []byte(in.Content), 0644); err != nil {
		return "", fmt.Errorf("writing %s: %w", in.FilePath, err)
	}

	// Update file state after successful write
	if f.FileState != nil {
		f.FileState.Set(in.FilePath, &tool.FileState{
			Content:   in.Content,
			Timestamp: tool.GetFileMtime(in.FilePath),
		})
	}

	return fmt.Sprintf("Successfully wrote to %s (%d bytes)", in.FilePath, len(in.Content)), nil
}
