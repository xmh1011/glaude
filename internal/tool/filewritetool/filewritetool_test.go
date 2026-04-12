package filewritetool

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

		input, _ := json.Marshal(Input{FilePath: path, Content: "hello world"})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Successfully wrote")

		data, _ := os.ReadFile(path)
		assert.Equal(t, "hello world", string(data))
	})

	t.Run("create with nested dirs", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "a", "b", "c", "deep.txt")

		input, _ := json.Marshal(Input{FilePath: path, Content: "deep content"})
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

		input, _ := json.Marshal(Input{FilePath: path, Content: "new"})
		_, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)

		data, _ := os.ReadFile(path)
		assert.Equal(t, "new", string(data))
	})
}
