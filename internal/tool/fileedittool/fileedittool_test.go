package fileedittool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileEditTool_Execute(t *testing.T) {
	tool := &FileEditTool{}

	t.Run("unique replace", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "test.txt")
		os.WriteFile(path, []byte("hello world\nfoo bar\n"), 0644)

		input, _ := json.Marshal(Input{
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

		input, _ := json.Marshal(Input{
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

		input, _ := json.Marshal(Input{
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

		input, _ := json.Marshal(Input{
			FilePath:  path,
			OldString: "missing",
			NewString: "replaced",
		})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})
}
