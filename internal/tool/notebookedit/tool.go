// Package notebookedit implements the NotebookEdit tool for Jupyter notebooks.
package notebookedit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Tool edits cells in Jupyter notebooks (.ipynb files).
type Tool struct{}

// Input is the parsed input for the NotebookEdit tool.
type Input struct {
	NotebookPath string  `json:"notebook_path"`
	CellID       *string `json:"cell_id,omitempty"`
	NewSource    string  `json:"new_source"`
	CellType     string  `json:"cell_type,omitempty"`
	EditMode     string  `json:"edit_mode,omitempty"` // replace, insert, delete
}

// Notebook represents the top-level structure of a .ipynb file.
type Notebook struct {
	Cells         []Cell         `json:"cells"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	NBFormat      int            `json:"nbformat"`
	NBFormatMinor int            `json:"nbformat_minor"`
}

// Cell represents a single cell in a notebook.
type Cell struct {
	ID         string         `json:"id,omitempty"`
	CellType   string         `json:"cell_type"`
	Source     []string       `json:"source"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Outputs    []any          `json:"outputs,omitempty"`
	ExecCount  *int           `json:"execution_count,omitempty"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "NotebookEdit" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Edits cells in Jupyter notebook (.ipynb) files. Supports replace, insert, and delete operations on individual cells."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"notebook_path": {"type": "string", "description": "Absolute path to the .ipynb file"},
			"cell_id": {"type": "string", "description": "The ID of the cell to edit. For insert, inserts after this cell."},
			"new_source": {"type": "string", "description": "The new source content for the cell"},
			"cell_type": {"type": "string", "enum": ["code", "markdown"], "description": "Cell type (required for insert)"},
			"edit_mode": {"type": "string", "enum": ["replace", "insert", "delete"], "description": "Edit operation (default: replace)"}
		},
		"required": ["notebook_path", "new_source"]
	}`)
}

// IsReadOnly returns false since this modifies files.
func (t *Tool) IsReadOnly() bool { return false }

// Execute edits a notebook cell.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if in.NotebookPath == "" {
		return "", fmt.Errorf("notebook_path is required")
	}

	mode := in.EditMode
	if mode == "" {
		mode = "replace"
	}

	data, err := os.ReadFile(in.NotebookPath)
	if err != nil {
		return "", fmt.Errorf("read notebook: %w", err)
	}

	var nb Notebook
	if err := json.Unmarshal(data, &nb); err != nil {
		return "", fmt.Errorf("parse notebook: %w", err)
	}

	// Convert source string to lines for the notebook format.
	sourceLines := splitSourceLines(in.NewSource)

	switch mode {
	case "replace":
		idx, err := findCell(&nb, in.CellID)
		if err != nil {
			return "", err
		}
		nb.Cells[idx].Source = sourceLines
		if in.CellType != "" {
			nb.Cells[idx].CellType = in.CellType
		}

	case "insert":
		cellType := in.CellType
		if cellType == "" {
			return "", fmt.Errorf("cell_type is required for insert mode")
		}
		newCell := Cell{
			CellType: cellType,
			Source:   sourceLines,
			Metadata: make(map[string]any),
		}
		if cellType == "code" {
			newCell.Outputs = []any{}
		}
		idx := 0
		if in.CellID != nil {
			after, err := findCell(&nb, in.CellID)
			if err != nil {
				return "", err
			}
			idx = after + 1
		}
		// Insert at idx.
		nb.Cells = append(nb.Cells[:idx], append([]Cell{newCell}, nb.Cells[idx:]...)...)

	case "delete":
		idx, err := findCell(&nb, in.CellID)
		if err != nil {
			return "", err
		}
		nb.Cells = append(nb.Cells[:idx], nb.Cells[idx+1:]...)

	default:
		return "", fmt.Errorf("unknown edit_mode: %q", mode)
	}

	out, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		return "", fmt.Errorf("marshal notebook: %w", err)
	}

	if err := os.WriteFile(in.NotebookPath, append(out, '\n'), 0644); err != nil {
		return "", fmt.Errorf("write notebook: %w", err)
	}

	return fmt.Sprintf("Notebook %s: %s operation completed on %s.", in.NotebookPath, mode, cellDesc(in.CellID)), nil
}

func findCell(nb *Notebook, cellID *string) (int, error) {
	if cellID == nil || *cellID == "" {
		if len(nb.Cells) == 0 {
			return 0, fmt.Errorf("notebook has no cells")
		}
		return 0, nil
	}

	for i, c := range nb.Cells {
		if c.ID == *cellID {
			return i, nil
		}
	}
	return 0, fmt.Errorf("cell with id %q not found", *cellID)
}

func splitSourceLines(s string) []string {
	if s == "" {
		return []string{}
	}
	lines := strings.Split(s, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		if i < len(lines)-1 {
			result[i] = line + "\n"
		} else {
			result[i] = line
		}
	}
	return result
}

func cellDesc(cellID *string) string {
	if cellID != nil && *cellID != "" {
		return fmt.Sprintf("cell %q", *cellID)
	}
	return "cell 0"
}
