package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileReadTool_Execute(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\nline4\nline5\n"), 0644)

	tool := &FileReadTool{}

	t.Run("read all", func(t *testing.T) {
		input, _ := json.Marshal(fileReadInput{FilePath: path})
		result, err := tool.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "line1") || !strings.Contains(result, "line5") {
			t.Fatalf("expected all lines, got:\n%s", result)
		}
		// Check line numbers
		if !strings.Contains(result, "     1\t") {
			t.Fatalf("expected line numbers, got:\n%s", result)
		}
	})

	t.Run("offset and limit", func(t *testing.T) {
		input, _ := json.Marshal(fileReadInput{FilePath: path, Offset: 2, Limit: 2})
		result, err := tool.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "line2") || !strings.Contains(result, "line3") {
			t.Fatalf("expected lines 2-3, got:\n%s", result)
		}
		if strings.Contains(result, "line1") || strings.Contains(result, "line4") {
			t.Fatalf("should not contain lines outside range, got:\n%s", result)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		input, _ := json.Marshal(fileReadInput{FilePath: "/nonexistent/file.txt"})
		_, err := tool.Execute(context.Background(), input)
		if err == nil {
			t.Fatal("expected error for missing file")
		}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "Successfully edited") {
			t.Fatalf("unexpected result: %s", result)
		}

		data, _ := os.ReadFile(path)
		if !strings.Contains(string(data), "baz qux") {
			t.Fatalf("expected replacement, got: %s", string(data))
		}
		if strings.Contains(string(data), "foo bar") {
			t.Fatalf("old string should be gone, got: %s", string(data))
		}
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
		if err == nil {
			t.Fatal("expected error for non-unique match")
		}
		if !strings.Contains(err.Error(), "2 times") {
			t.Fatalf("expected count in error, got: %v", err)
		}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "3 occurrences") {
			t.Fatalf("expected count in result, got: %s", result)
		}

		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), "aaa") {
			t.Fatalf("all occurrences should be replaced, got: %s", string(data))
		}
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
		if err == nil {
			t.Fatal("expected error for not found")
		}
	})
}
