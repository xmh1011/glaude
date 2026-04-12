package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderDiff_NoChanges(t *testing.T) {
	result := RenderDiff("test.go", "hello world", "hello world")
	assert.Empty(t, result, "identical content should produce empty diff")
}

func TestRenderDiff_WithChanges(t *testing.T) {
	result := RenderDiff("test.go", "line one\nline two\n", "line one\nline changed\n")
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "test.go")
}

func TestRenderUnifiedDiff_NoChanges(t *testing.T) {
	result := RenderUnifiedDiff("test.go", "same content", "same content")
	assert.Empty(t, result)
}

func TestRenderUnifiedDiff_WithChanges(t *testing.T) {
	old := "func main() {\n\tfmt.Println(\"hello\")\n}\n"
	new := "func main() {\n\tfmt.Println(\"world\")\n}\n"
	result := RenderUnifiedDiff("main.go", old, new)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "main.go")
}
