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

func TestGlobTool_Execute(t *testing.T) {
	// Create test directory structure
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "src"), 0755)
	os.MkdirAll(filepath.Join(tmp, ".git", "objects"), 0755)
	os.MkdirAll(filepath.Join(tmp, "node_modules", "pkg"), 0755)
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmp, "src", "app.go"), []byte("package src"), 0644)
	os.WriteFile(filepath.Join(tmp, "src", "app_test.go"), []byte("package src"), 0644)
	os.WriteFile(filepath.Join(tmp, "readme.md"), []byte("# readme"), 0644)
	os.WriteFile(filepath.Join(tmp, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)
	os.WriteFile(filepath.Join(tmp, "node_modules", "pkg", "index.js"), []byte(""), 0644)

	tool := &GlobTool{}

	t.Run("match go files", func(t *testing.T) {
		input, _ := json.Marshal(globInput{Pattern: "*.go", Path: tmp})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "main.go")
		assert.Contains(t, result, "app.go")
		assert.Contains(t, result, "app_test.go")
	})

	t.Run("excludes .git", func(t *testing.T) {
		input, _ := json.Marshal(globInput{Pattern: "*", Path: tmp})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.NotContains(t, result, ".git")
		assert.NotContains(t, result, "HEAD")
	})

	t.Run("excludes node_modules", func(t *testing.T) {
		input, _ := json.Marshal(globInput{Pattern: "*.js", Path: tmp})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.NotContains(t, result, "node_modules")
		assert.NotContains(t, result, "index.js")
	})

	t.Run("no matches", func(t *testing.T) {
		input, _ := json.Marshal(globInput{Pattern: "*.xyz", Path: tmp})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "No files matched")
	})

	t.Run("double star pattern", func(t *testing.T) {
		input, _ := json.Marshal(globInput{Pattern: "**/*.go", Path: tmp})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "main.go")
		assert.Contains(t, result, "app.go")
	})
}
