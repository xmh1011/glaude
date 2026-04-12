package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileStore_Load(t *testing.T) {
	store := &FileStore{}

	t.Run("no files exist", func(t *testing.T) {
		tmp := t.TempDir()
		content, err := store.Load(tmp)
		require.NoError(t, err)
		assert.Empty(t, content)
	})

	t.Run("project GLAUDE.md only", func(t *testing.T) {
		tmp := t.TempDir()
		os.WriteFile(filepath.Join(tmp, "GLAUDE.md"), []byte("project rules"), 0644)

		content, err := store.Load(tmp)
		require.NoError(t, err)
		assert.Contains(t, content, "project rules")
	})

	t.Run("multiple tiers merged", func(t *testing.T) {
		tmp := t.TempDir()
		os.WriteFile(filepath.Join(tmp, "GLAUDE.md"), []byte("project rules"), 0644)
		os.MkdirAll(filepath.Join(tmp, ".glaude"), 0755)
		os.WriteFile(filepath.Join(tmp, ".glaude", "GLAUDE.md"), []byte("dotdir rules"), 0644)
		os.WriteFile(filepath.Join(tmp, "GLAUDE.local.md"), []byte("local overrides"), 0644)

		content, err := store.Load(tmp)
		require.NoError(t, err)
		assert.Contains(t, content, "project rules")
		assert.Contains(t, content, "dotdir rules")
		assert.Contains(t, content, "local overrides")

		// Local should appear after project (higher priority = later position)
		idxProject := strings.Index(content, "project rules")
		idxLocal := strings.Index(content, "local overrides")
		assert.Greater(t, idxLocal, idxProject, "local should come after project")
	})

	t.Run("whitespace trimmed", func(t *testing.T) {
		tmp := t.TempDir()
		os.WriteFile(filepath.Join(tmp, "GLAUDE.md"), []byte("  trimmed  \n\n"), 0644)

		content, err := store.Load(tmp)
		require.NoError(t, err)
		assert.Equal(t, "trimmed", content)
	})
}

func TestFileStore_Save(t *testing.T) {
	store := &FileStore{}

	t.Run("save creates file", func(t *testing.T) {
		tmp := t.TempDir()
		err := store.Save(tmp, "saved content")
		require.NoError(t, err)

		data, _ := os.ReadFile(filepath.Join(tmp, "GLAUDE.md"))
		assert.Equal(t, "saved content", string(data))
	})

	t.Run("save overwrites existing", func(t *testing.T) {
		tmp := t.TempDir()
		os.WriteFile(filepath.Join(tmp, "GLAUDE.md"), []byte("old"), 0644)

		err := store.Save(tmp, "new")
		require.NoError(t, err)

		data, _ := os.ReadFile(filepath.Join(tmp, "GLAUDE.md"))
		assert.Equal(t, "new", string(data))
	})
}
