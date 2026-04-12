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

func TestFileReadTool_Execute(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\nline4\nline5\n"), 0644)

	tool := &FileReadTool{}

	t.Run("read all", func(t *testing.T) {
		input, _ := json.Marshal(fileReadInput{FilePath: path})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "line1")
		assert.Contains(t, result, "line5")
		assert.Contains(t, result, "     1\t", "expected line numbers")
	})

	t.Run("offset and limit", func(t *testing.T) {
		input, _ := json.Marshal(fileReadInput{FilePath: path, Offset: 2, Limit: 2})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "line2")
		assert.Contains(t, result, "line3")
		assert.NotContains(t, result, "line1")
		assert.NotContains(t, result, "line4")
	})

	t.Run("missing file", func(t *testing.T) {
		input, _ := json.Marshal(fileReadInput{FilePath: "/nonexistent/file.txt"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})
}
