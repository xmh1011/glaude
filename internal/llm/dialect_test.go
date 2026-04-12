package llm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeParseJSON_Valid(t *testing.T) {
	input := []byte(`{"key": "value"}`)
	result := SafeParseJSON(input)
	assert.True(t, json.Valid(result))
	assert.JSONEq(t, `{"key":"value"}`, string(result))
}

func TestSafeParseJSON_BOM(t *testing.T) {
	input := append([]byte("\xef\xbb\xbf"), []byte(`{"key": "value"}`)...)
	result := SafeParseJSON(input)
	assert.True(t, json.Valid(result))
}

func TestSafeParseJSON_TrailingComma(t *testing.T) {
	input := []byte(`{"key": "value", "list": [1, 2, 3,],}`)
	result := SafeParseJSON(input)
	assert.True(t, json.Valid(result), "result should be valid JSON: %s", string(result))
}

func TestSafeParseJSON_UnclosedBrace(t *testing.T) {
	input := []byte(`{"command": "ls -la"`)
	result := SafeParseJSON(input)
	assert.True(t, json.Valid(result), "result should be valid JSON: %s", string(result))

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(result, &parsed))
	assert.Equal(t, "ls -la", parsed["command"])
}

func TestSafeParseJSON_UnclosedBracket(t *testing.T) {
	input := []byte(`{"items": [1, 2, 3`)
	result := SafeParseJSON(input)
	assert.True(t, json.Valid(result), "result should be valid JSON: %s", string(result))
}

func TestSafeParseJSON_Empty(t *testing.T) {
	result := SafeParseJSON([]byte(""))
	assert.Equal(t, `{}`, string(result))
}

func TestSafeParseJSON_Garbage(t *testing.T) {
	result := SafeParseJSON([]byte("not json at all"))
	assert.Equal(t, `{}`, string(result))
}

func TestSafeParseJSON_NestedUnclosed(t *testing.T) {
	input := []byte(`{"a": {"b": [1, 2`)
	result := SafeParseJSON(input)
	assert.True(t, json.Valid(result), "result should be valid JSON: %s", string(result))
}

func TestDialectFixer_RepairsToolInput(t *testing.T) {
	mock := NewMockProvider(
		&Response{
			Content: []ContentBlock{
				{
					Type:  ContentToolUse,
					ID:    "call_1",
					Name:  "Bash",
					Input: json.RawMessage(`{"command": "ls",}`), // trailing comma
				},
			},
			StopReason: StopToolUse,
			Usage:      Usage{InputTokens: 10, OutputTokens: 5},
		},
	)

	fixer := NewDialectFixer(mock)
	resp, err := fixer.Complete(context.Background(), &Request{
		Model:     "test",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("ls")}}},
		MaxTokens: 1024,
	})

	require.NoError(t, err)
	require.Len(t, resp.Content, 1)
	assert.True(t, json.Valid(resp.Content[0].Input), "input should be valid JSON after fix")
}

func TestDialectFixer_FiltersEmptyToolCalls(t *testing.T) {
	mock := NewMockProvider(
		&Response{
			Content: []ContentBlock{
				NewTextBlock("thinking..."),
				{Type: ContentToolUse, ID: "call_1", Name: "", Input: json.RawMessage(`{}`)}, // empty name
				{Type: ContentToolUse, ID: "call_2", Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)},
			},
			StopReason: StopToolUse,
			Usage:      Usage{InputTokens: 10, OutputTokens: 5},
		},
	)

	fixer := NewDialectFixer(mock)
	resp, err := fixer.Complete(context.Background(), &Request{
		Model:     "test",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("ls")}}},
		MaxTokens: 1024,
	})

	require.NoError(t, err)
	assert.Len(t, resp.Content, 2, "empty tool call should be filtered out")
	assert.Equal(t, ContentText, resp.Content[0].Type)
	assert.Equal(t, "Bash", resp.Content[1].Name)
}

func TestDialectFixer_EmptyInput(t *testing.T) {
	mock := NewMockProvider(
		&Response{
			Content: []ContentBlock{
				{Type: ContentToolUse, ID: "call_1", Name: "Read", Input: nil},
			},
			StopReason: StopToolUse,
			Usage:      Usage{InputTokens: 10, OutputTokens: 5},
		},
	)

	fixer := NewDialectFixer(mock)
	resp, err := fixer.Complete(context.Background(), &Request{
		Model:     "test",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("read")}}},
		MaxTokens: 1024,
	})

	require.NoError(t, err)
	require.Len(t, resp.Content, 1)
	assert.Equal(t, `{}`, string(resp.Content[0].Input), "nil input should become {}")
}

func TestDialectFixer_PassthroughErrors(t *testing.T) {
	mock := &dialectErrorProvider{err: assert.AnError}
	fixer := NewDialectFixer(mock)
	_, err := fixer.Complete(context.Background(), &Request{
		Model:     "test",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("hi")}}},
		MaxTokens: 1024,
	})
	assert.Error(t, err)
}

// dialectErrorProvider always returns an error (for dialect tests).
type dialectErrorProvider struct {
	err error
}

func (e *dialectErrorProvider) Complete(_ context.Context, _ *Request) (*Response, error) {
	return nil, e.err
}
