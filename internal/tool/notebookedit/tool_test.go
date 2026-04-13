package notebookedit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTool_Metadata(t *testing.T) {
	tool := &Tool{}
	assert.Equal(t, "NotebookEdit", tool.Name())
	assert.False(t, tool.IsReadOnly())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
}

const sampleNotebook = `{
 "cells": [
  {"id": "cell1", "cell_type": "code", "source": ["print('hello')\n"], "metadata": {}, "outputs": [], "execution_count": null},
  {"id": "cell2", "cell_type": "markdown", "source": ["# Title\n"], "metadata": {}}
 ],
 "metadata": {},
 "nbformat": 4,
 "nbformat_minor": 5
}`

func writeSampleNotebook(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ipynb")
	require.NoError(t, os.WriteFile(path, []byte(sampleNotebook), 0644))
	return path
}

func TestTool_Execute(t *testing.T) {
	tool := &Tool{}

	t.Run("replace cell", func(t *testing.T) {
		path := writeSampleNotebook(t)
		cellID := "cell1"
		input, _ := json.Marshal(Input{
			NotebookPath: path,
			CellID:       &cellID,
			NewSource:    "print('world')",
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "replace")

		// Verify the change was written.
		data, _ := os.ReadFile(path)
		assert.Contains(t, string(data), "world")
	})

	t.Run("insert cell", func(t *testing.T) {
		path := writeSampleNotebook(t)
		cellID := "cell1"
		input, _ := json.Marshal(Input{
			NotebookPath: path,
			CellID:       &cellID,
			NewSource:    "x = 42",
			CellType:     "code",
			EditMode:     "insert",
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "insert")

		data, _ := os.ReadFile(path)
		var nb Notebook
		json.Unmarshal(data, &nb)
		require.Len(t, nb.Cells, 3)
	})

	t.Run("delete cell", func(t *testing.T) {
		path := writeSampleNotebook(t)
		cellID := "cell2"
		input, _ := json.Marshal(Input{
			NotebookPath: path,
			CellID:       &cellID,
			NewSource:    "",
			EditMode:     "delete",
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "delete")

		data, _ := os.ReadFile(path)
		var nb Notebook
		json.Unmarshal(data, &nb)
		require.Len(t, nb.Cells, 1)
	})

	t.Run("cell not found", func(t *testing.T) {
		path := writeSampleNotebook(t)
		cellID := "nonexistent"
		input, _ := json.Marshal(Input{
			NotebookPath: path,
			CellID:       &cellID,
			NewSource:    "test",
		})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("file not found", func(t *testing.T) {
		input, _ := json.Marshal(Input{
			NotebookPath: "/nonexistent/path.ipynb",
			NewSource:    "test",
		})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})

	t.Run("insert requires cell_type", func(t *testing.T) {
		path := writeSampleNotebook(t)
		input, _ := json.Marshal(Input{
			NotebookPath: path,
			NewSource:    "test",
			EditMode:     "insert",
		})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cell_type")
	})
}
