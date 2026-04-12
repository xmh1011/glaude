package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"glaude/internal/memory"
)

// FileEditTool performs precise string replacement in files.
// It uses str_replace semantics: old_string must match exactly once
// (unless replace_all is set), then is replaced with new_string.
type FileEditTool struct {
	Checkpoint *memory.Checkpoint
}

type fileEditInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

func (f *FileEditTool) Name() string { return "Edit" }

func (f *FileEditTool) Description() string {
	return "Performs exact string replacements in files. The old_string must be unique in the file unless replace_all is set."
}

func (f *FileEditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {"type": "string", "description": "The absolute path to the file to modify"},
			"old_string": {"type": "string", "description": "The text to find and replace"},
			"new_string": {"type": "string", "description": "The replacement text"},
			"replace_all": {"type": "boolean", "description": "Replace all occurrences (default false)"}
		},
		"required": ["file_path", "old_string", "new_string"]
	}`)
}

func (f *FileEditTool) IsReadOnly() bool { return false }

func (f *FileEditTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in fileEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}
	if in.OldString == "" {
		return "", fmt.Errorf("old_string is required")
	}
	if in.OldString == in.NewString {
		return "", fmt.Errorf("old_string and new_string must be different")
	}

	data, err := os.ReadFile(in.FilePath)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", in.FilePath, err)
	}

	content := string(data)
	count := strings.Count(content, in.OldString)

	if count == 0 {
		return "", fmt.Errorf("old_string not found in %s", in.FilePath)
	}
	if count > 1 && !in.ReplaceAll {
		return "", fmt.Errorf("old_string found %d times in %s (expected exactly 1; use replace_all to replace all)", count, in.FilePath)
	}

	var newContent string
	if in.ReplaceAll {
		newContent = strings.ReplaceAll(content, in.OldString, in.NewString)
	} else {
		newContent = strings.Replace(content, in.OldString, in.NewString, 1)
	}

	// Checkpoint: save file state before mutation
	if f.Checkpoint != nil {
		txID := f.Checkpoint.NextTxID()
		if err := f.Checkpoint.Save(txID, in.FilePath); err != nil {
			return "", fmt.Errorf("checkpoint: %w", err)
		}
	}

	// Preserve original file permissions
	info, err := os.Stat(in.FilePath)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", in.FilePath, err)
	}

	if err := os.WriteFile(in.FilePath, []byte(newContent), info.Mode()); err != nil {
		return "", fmt.Errorf("writing %s: %w", in.FilePath, err)
	}

	if in.ReplaceAll {
		return fmt.Sprintf("Replaced %d occurrences in %s", count, in.FilePath), nil
	}
	return fmt.Sprintf("Successfully edited %s", in.FilePath), nil
}
