package greptool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrepTool_Execute(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "src"), 0755)
	os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
	os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"), 0644)
	os.WriteFile(filepath.Join(tmp, "src", "app.go"), []byte("package src\n\nfunc App() string {\n\treturn \"app\"\n}\n"), 0644)
	os.WriteFile(filepath.Join(tmp, ".git", "config"), []byte("package hidden\n"), 0644)

	tool := &GrepTool{}

	t.Run("files_with_matches mode", func(t *testing.T) {
		input, _ := json.Marshal(map[string]interface{}{
			"pattern":     "package",
			"path":        tmp,
			"output_mode": "files_with_matches",
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "main.go")
		assert.Contains(t, result, "app.go")
		// Should not include .git
		assert.NotContains(t, result, ".git")
	})

	t.Run("content mode", func(t *testing.T) {
		input, _ := json.Marshal(map[string]interface{}{
			"pattern":     "func",
			"path":        tmp,
			"output_mode": "content",
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "func")
		// Should show line numbers
		lines := strings.Split(result, "\n")
		hasLineNum := false
		for _, l := range lines {
			if strings.Contains(l, ":") && len(l) > 0 {
				hasLineNum = true
				break
			}
		}
		assert.True(t, hasLineNum, "content mode should include line numbers")
	})

	t.Run("no matches", func(t *testing.T) {
		input, _ := json.Marshal(map[string]interface{}{
			"pattern": "nonexistent_xyz_pattern",
			"path":    tmp,
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "No matches")
	})

	t.Run("glob filter", func(t *testing.T) {
		input, _ := json.Marshal(map[string]interface{}{
			"pattern": "package",
			"path":    tmp,
			"glob":    "*.go",
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, ".go")
	})
}
