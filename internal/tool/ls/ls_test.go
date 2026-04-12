package ls

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTool_Execute(t *testing.T) {
	tool := &Tool{}

	t.Run("list directory", func(t *testing.T) {
		tmp := t.TempDir()
		os.MkdirAll(filepath.Join(tmp, "subdir"), 0755)
		os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)

		input, _ := json.Marshal(Input{Path: tmp})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "file.txt")
		assert.Contains(t, result, "subdir/")
		assert.Contains(t, result, "<dir>")
	})

	t.Run("empty directory", func(t *testing.T) {
		tmp := t.TempDir()

		input, _ := json.Marshal(Input{Path: tmp})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "empty directory")
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		input, _ := json.Marshal(Input{Path: "/nonexistent_xyz"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})
}
