package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckpoint_SaveAndUndo(t *testing.T) {
	t.Run("undo restores file content", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "file.txt")
		os.WriteFile(path, []byte("original"), 0644)

		cp := NewCheckpoint()
		require.NoError(t, cp.Save("tx-1", path))

		// Simulate a write that changes the file
		os.WriteFile(path, []byte("modified"), 0644)

		txID, err := cp.Undo()
		require.NoError(t, err)
		assert.Equal(t, "tx-1", txID)

		data, _ := os.ReadFile(path)
		assert.Equal(t, "original", string(data))
	})

	t.Run("undo removes newly created file", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "new.txt")

		cp := NewCheckpoint()
		require.NoError(t, cp.Save("tx-2", path))

		// Simulate file creation
		os.WriteFile(path, []byte("created"), 0644)

		_, err := cp.Undo()
		require.NoError(t, err)

		_, err = os.Stat(path)
		assert.True(t, os.IsNotExist(err), "file should be removed after undo")
	})

	t.Run("undo empty stack returns error", func(t *testing.T) {
		cp := NewCheckpoint()
		_, err := cp.Undo()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nothing to undo")
	})

	t.Run("multiple transactions undo in reverse order", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "file.txt")
		os.WriteFile(path, []byte("v1"), 0644)

		cp := NewCheckpoint()

		// First transaction
		require.NoError(t, cp.Save("tx-a", path))
		os.WriteFile(path, []byte("v2"), 0644)

		// Second transaction
		require.NoError(t, cp.Save("tx-b", path))
		os.WriteFile(path, []byte("v3"), 0644)

		assert.Equal(t, 2, cp.Len())

		// Undo tx-b: should restore v2
		txID, err := cp.Undo()
		require.NoError(t, err)
		assert.Equal(t, "tx-b", txID)
		data, _ := os.ReadFile(path)
		assert.Equal(t, "v2", string(data))

		// Undo tx-a: should restore v1
		txID, err = cp.Undo()
		require.NoError(t, err)
		assert.Equal(t, "tx-a", txID)
		data, _ = os.ReadFile(path)
		assert.Equal(t, "v1", string(data))

		assert.Equal(t, 0, cp.Len())
	})

	t.Run("same txID groups into one transaction", func(t *testing.T) {
		tmp := t.TempDir()
		pathA := filepath.Join(tmp, "a.txt")
		pathB := filepath.Join(tmp, "b.txt")
		os.WriteFile(pathA, []byte("a-original"), 0644)
		os.WriteFile(pathB, []byte("b-original"), 0644)

		cp := NewCheckpoint()
		require.NoError(t, cp.Save("tx-group", pathA))
		require.NoError(t, cp.Save("tx-group", pathB))

		// Only one transaction
		assert.Equal(t, 1, cp.Len())

		// Modify both files
		os.WriteFile(pathA, []byte("a-modified"), 0644)
		os.WriteFile(pathB, []byte("b-modified"), 0644)

		// Undo restores both
		_, err := cp.Undo()
		require.NoError(t, err)

		dataA, _ := os.ReadFile(pathA)
		dataB, _ := os.ReadFile(pathB)
		assert.Equal(t, "a-original", string(dataA))
		assert.Equal(t, "b-original", string(dataB))
	})

	t.Run("peek returns latest transaction ID", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "file.txt")
		os.WriteFile(path, []byte("data"), 0644)

		cp := NewCheckpoint()
		assert.Equal(t, "", cp.Peek())

		require.NoError(t, cp.Save("tx-peek", path))
		assert.Equal(t, "tx-peek", cp.Peek())
	})

	t.Run("preserves file permissions", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "exec.sh")
		os.WriteFile(path, []byte("#!/bin/sh"), 0755)

		cp := NewCheckpoint()
		require.NoError(t, cp.Save("tx-perm", path))

		os.WriteFile(path, []byte("modified"), 0644)

		_, err := cp.Undo()
		require.NoError(t, err)

		info, _ := os.Stat(path)
		assert.Equal(t, os.FileMode(0755), info.Mode().Perm())

		data, _ := os.ReadFile(path)
		assert.Equal(t, "#!/bin/sh", string(data))
	})
}
