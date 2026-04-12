package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileWriteTool_Execute(t *testing.T) {
	tool := &FileWriteTool{}

	t.Run("create new file", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "new.txt")

		input, _ := json.Marshal(fileWriteInput{FilePath: path, Content: "hello world"})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Successfully wrote")

		data, _ := os.ReadFile(path)
		assert.Equal(t, "hello world", string(data))
	})

	t.Run("create with nested dirs", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "a", "b", "c", "deep.txt")

		input, _ := json.Marshal(fileWriteInput{FilePath: path, Content: "deep content"})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Successfully wrote")

		data, _ := os.ReadFile(path)
		assert.Equal(t, "deep content", string(data))
	})

	t.Run("overwrite existing", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "exist.txt")
		os.WriteFile(path, []byte("old"), 0644)

		input, _ := json.Marshal(fileWriteInput{FilePath: path, Content: "new"})
		_, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)

		data, _ := os.ReadFile(path)
		assert.Equal(t, "new", string(data))
	})
}

func TestLSTool_Execute(t *testing.T) {
	tool := &LSTool{}

	t.Run("list directory", func(t *testing.T) {
		tmp := t.TempDir()
		os.MkdirAll(filepath.Join(tmp, "subdir"), 0755)
		os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)

		input, _ := json.Marshal(lsInput{Path: tmp})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "file.txt")
		assert.Contains(t, result, "subdir/")
		assert.Contains(t, result, "<dir>")
	})

	t.Run("empty directory", func(t *testing.T) {
		tmp := t.TempDir()

		input, _ := json.Marshal(lsInput{Path: tmp})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "empty directory")
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		input, _ := json.Marshal(lsInput{Path: "/nonexistent_xyz"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})
}
