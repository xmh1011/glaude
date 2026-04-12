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

func TestFileStore_RulesDir(t *testing.T) {
	store := &FileStore{}

	t.Run("loads rules from .glaude/rules/", func(t *testing.T) {
		tmp := t.TempDir()
		rulesDir := filepath.Join(tmp, ".glaude", "rules")
		os.MkdirAll(rulesDir, 0755)
		os.WriteFile(filepath.Join(rulesDir, "style.md"), []byte("use tabs"), 0644)
		os.WriteFile(filepath.Join(rulesDir, "naming.md"), []byte("use camelCase"), 0644)

		content, err := store.Load(tmp)
		require.NoError(t, err)
		assert.Contains(t, content, "use tabs")
		assert.Contains(t, content, "use camelCase")
	})

	t.Run("rules sorted alphabetically", func(t *testing.T) {
		tmp := t.TempDir()
		rulesDir := filepath.Join(tmp, ".glaude", "rules")
		os.MkdirAll(rulesDir, 0755)
		os.WriteFile(filepath.Join(rulesDir, "z-last.md"), []byte("last rule"), 0644)
		os.WriteFile(filepath.Join(rulesDir, "a-first.md"), []byte("first rule"), 0644)

		content, err := store.Load(tmp)
		require.NoError(t, err)
		idxFirst := strings.Index(content, "first rule")
		idxLast := strings.Index(content, "last rule")
		assert.Less(t, idxFirst, idxLast, "rules should be sorted alphabetically")
	})

	t.Run("ignores non-md files", func(t *testing.T) {
		tmp := t.TempDir()
		rulesDir := filepath.Join(tmp, ".glaude", "rules")
		os.MkdirAll(rulesDir, 0755)
		os.WriteFile(filepath.Join(rulesDir, "rule.md"), []byte("valid rule"), 0644)
		os.WriteFile(filepath.Join(rulesDir, "data.json"), []byte("not a rule"), 0644)

		content, err := store.Load(tmp)
		require.NoError(t, err)
		assert.Contains(t, content, "valid rule")
		assert.NotContains(t, content, "not a rule")
	})
}

func TestFileStore_Include(t *testing.T) {
	t.Run("@include with relative path", func(t *testing.T) {
		tmp := t.TempDir()
		os.WriteFile(filepath.Join(tmp, "extra.md"), []byte("included content"), 0644)
		os.WriteFile(filepath.Join(tmp, "GLAUDE.md"), []byte("main rules\n@./extra.md"), 0644)

		store := &FileStore{}
		content, err := store.Load(tmp)
		require.NoError(t, err)
		assert.Contains(t, content, "main rules")
		assert.Contains(t, content, "included content")
	})

	t.Run("@include bare path", func(t *testing.T) {
		tmp := t.TempDir()
		os.WriteFile(filepath.Join(tmp, "extra.md"), []byte("bare included"), 0644)
		os.WriteFile(filepath.Join(tmp, "GLAUDE.md"), []byte("rules\n@extra.md"), 0644)

		store := &FileStore{}
		content, err := store.Load(tmp)
		require.NoError(t, err)
		assert.Contains(t, content, "bare included")
	})

	t.Run("circular reference prevented", func(t *testing.T) {
		tmp := t.TempDir()
		// a.md includes b.md, b.md includes a.md
		os.WriteFile(filepath.Join(tmp, "a.md"), []byte("alpha\n@./b.md"), 0644)
		os.WriteFile(filepath.Join(tmp, "b.md"), []byte("beta\n@./a.md"), 0644)
		os.WriteFile(filepath.Join(tmp, "GLAUDE.md"), []byte("root\n@./a.md"), 0644)

		store := &FileStore{}
		content, err := store.Load(tmp)
		require.NoError(t, err)
		assert.Contains(t, content, "alpha")
		assert.Contains(t, content, "beta")
		// Should not infinite loop — passes if we get here
	})

	t.Run("depth limit respected", func(t *testing.T) {
		tmp := t.TempDir()
		// Create a chain: GLAUDE.md -> d1.md -> d2.md -> d3.md -> d4.md -> d5.md -> d6.md
		os.WriteFile(filepath.Join(tmp, "d6.md"), []byte("depth6"), 0644)
		os.WriteFile(filepath.Join(tmp, "d5.md"), []byte("depth5\n@./d6.md"), 0644)
		os.WriteFile(filepath.Join(tmp, "d4.md"), []byte("depth4\n@./d5.md"), 0644)
		os.WriteFile(filepath.Join(tmp, "d3.md"), []byte("depth3\n@./d4.md"), 0644)
		os.WriteFile(filepath.Join(tmp, "d2.md"), []byte("depth2\n@./d3.md"), 0644)
		os.WriteFile(filepath.Join(tmp, "d1.md"), []byte("depth1\n@./d2.md"), 0644)
		os.WriteFile(filepath.Join(tmp, "GLAUDE.md"), []byte("root\n@./d1.md"), 0644)

		store := &FileStore{}
		content, err := store.Load(tmp)
		require.NoError(t, err)
		assert.Contains(t, content, "depth1")
		assert.Contains(t, content, "depth5")
		// depth6 should NOT be included (maxIncludeDepth=5)
		assert.NotContains(t, content, "depth6")
	})

	t.Run("non-text files skipped", func(t *testing.T) {
		tmp := t.TempDir()
		os.WriteFile(filepath.Join(tmp, "image.png"), []byte("binary data"), 0644)
		os.WriteFile(filepath.Join(tmp, "GLAUDE.md"), []byte("rules\n@./image.png"), 0644)

		store := &FileStore{}
		content, err := store.Load(tmp)
		require.NoError(t, err)
		assert.NotContains(t, content, "binary data")
	})

	t.Run("fragment identifiers stripped", func(t *testing.T) {
		tmp := t.TempDir()
		os.WriteFile(filepath.Join(tmp, "doc.md"), []byte("full doc content"), 0644)
		os.WriteFile(filepath.Join(tmp, "GLAUDE.md"), []byte("rules\n@./doc.md#section"), 0644)

		store := &FileStore{}
		content, err := store.Load(tmp)
		require.NoError(t, err)
		assert.Contains(t, content, "full doc content")
	})
}

func TestStripHTMLComments(t *testing.T) {
	t.Run("no comments", func(t *testing.T) {
		input := "hello world"
		assert.Equal(t, "hello world", stripHTMLComments(input))
	})

	t.Run("single line comment", func(t *testing.T) {
		input := "before <!-- hidden --> after"
		result := stripHTMLComments(input)
		assert.Contains(t, result, "before")
		assert.Contains(t, result, "after")
		assert.NotContains(t, result, "hidden")
	})

	t.Run("multi-line comment", func(t *testing.T) {
		input := "visible\n<!-- \nhidden\ncontent\n-->\nmore visible"
		result := stripHTMLComments(input)
		assert.Contains(t, result, "visible")
		assert.Contains(t, result, "more visible")
		assert.NotContains(t, result, "hidden content")
	})

	t.Run("preserves comments in code blocks", func(t *testing.T) {
		input := "```\n<!-- code comment -->\n```\n<!-- stripped -->\ntext"
		result := stripHTMLComments(input)
		assert.Contains(t, result, "<!-- code comment -->")
		assert.NotContains(t, result, "<!-- stripped -->")
	})
}

func TestIsTextFile(t *testing.T) {
	assert.True(t, isTextFile("file.md"))
	assert.True(t, isTextFile("file.go"))
	assert.True(t, isTextFile("file.py"))
	assert.True(t, isTextFile("file.ts"))
	assert.True(t, isTextFile("file.json"))
	assert.False(t, isTextFile("file.png"))
	assert.False(t, isTextFile("file.jpg"))
	assert.False(t, isTextFile("file.exe"))
	assert.False(t, isTextFile("file.pdf"))
}
