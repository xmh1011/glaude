package websearch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/llm"
)

type mockProvider struct {
	response string
	err      error
}

func (m *mockProvider) Complete(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.Response{
		Content:    []llm.ContentBlock{llm.NewTextBlock(m.response)},
		StopReason: llm.StopEndTurn,
	}, nil
}

func TestTool_Metadata(t *testing.T) {
	tool := &Tool{Provider: &mockProvider{}, Model: "test"}
	assert.Equal(t, "WebSearch", tool.Name())
	assert.True(t, tool.IsReadOnly())

	var schema map[string]any
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
}

func TestTool_Execute(t *testing.T) {
	t.Run("successful search", func(t *testing.T) {
		tool := &Tool{
			Provider: &mockProvider{response: "Here are the search results..."},
			Model:    "test",
		}
		input, _ := json.Marshal(Input{Query: "golang testing"})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, result, "search results")
	})

	t.Run("with domains", func(t *testing.T) {
		tool := &Tool{
			Provider: &mockProvider{response: "Filtered results"},
			Model:    "test",
		}
		input, _ := json.Marshal(Input{
			Query:          "golang",
			AllowedDomains: []string{"golang.org"},
			BlockedDomains: []string{"stackoverflow.com"},
		})
		result, err := tool.Execute(context.Background(), input)
		require.NoError(t, err)
		assert.NotEmpty(t, result)
	})

	t.Run("missing query", func(t *testing.T) {
		tool := &Tool{Provider: &mockProvider{}, Model: "test"}
		input, _ := json.Marshal(Input{})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})

	t.Run("provider error", func(t *testing.T) {
		tool := &Tool{
			Provider: &mockProvider{err: assert.AnError},
			Model:    "test",
		}
		input, _ := json.Marshal(Input{Query: "test"})
		_, err := tool.Execute(context.Background(), input)
		assert.Error(t, err)
	})
}
