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

func TestFileEditTool_Execute(t *testing.T) {
	tool := &FileEditTool{}

	t.Run("unique replace", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "test.txt")
		os.WriteFile(path, []byte("hello world\nfoo bar\n"), 0644)

		input, _ := json.Marshal(fileEditInput{
			FilePath:  path,
			OldString: "foo bar",
			NewString: "baz qux",
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "Successfully edited")

		data, _ := os.ReadFile(path)
		assert.Contains(t, string(data), "baz qux")
		assert.NotContains(t, string(data), "foo bar")
	})

	t.Run("non-unique without replace_all", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "test.txt")
		os.WriteFile(path, []byte("aaa\naaa\n"), 0644)

		input, _ := json.Marshal(fileEditInput{
			FilePath:  path,
			OldString: "aaa",
			NewString: "bbb",
		})
		_, err := tool.Execute(context.Background(), input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "2 times")
	})

	t.Run("replace_all", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "test.txt")
		os.WriteFile(path, []byte("aaa\naaa\naaa\n"), 0644)

		input, _ := json.Marshal(fileEditInput{
			FilePath:   path,
			OldString:  "aaa",
			NewString:  "bbb",
			ReplaceAll: true,
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "3 occurrences")

		data, _ := os.ReadFile(path)
		assert.NotContains(t, string(data), "aaa")
	})

	t.Run("old_string not found", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "test.txt")
		os.WriteFile(path, []byte("hello\n"), 0644)

		input, _ := json.Marshal(fileEditInput{
			FilePath:  path,
			OldString: "missing",
			NewString: "replaced",
		})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})
}
