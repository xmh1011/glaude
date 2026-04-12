package compact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/llm"
)

func TestTokenCounter_Count(t *testing.T) {
	c := NewTokenCounter()

	t.Run("empty string", func(t *testing.T) {
		assert.Equal(t, 0, c.Count(""))
	})

	t.Run("simple text", func(t *testing.T) {
		count := c.Count("Hello, world!")
		assert.Greater(t, count, 0)
		assert.Less(t, count, 10) // should be ~4 tokens
	})

	t.Run("longer text", func(t *testing.T) {
		text := "This is a longer text that should have more tokens than a simple greeting."
		count := c.Count(text)
		assert.Greater(t, count, 10)
	})
}

func TestTokenCounter_CountMessages(t *testing.T) {
	c := NewTokenCounter()

	messages := []llm.Message{
		{
			Role:    llm.RoleUser,
			Content: []llm.ContentBlock{llm.NewTextBlock("Hello")},
		},
		{
			Role:    llm.RoleAssistant,
			Content: []llm.ContentBlock{llm.NewTextBlock("Hi there!")},
		},
	}

	count := c.CountMessages(messages)
	assert.Greater(t, count, 0)
}

func TestBudget_Calculations(t *testing.T) {
	b := NewBudget(200000, 16000)

	t.Run("effective window", func(t *testing.T) {
		assert.Equal(t, 184000, b.EffectiveWindow())
	})

	t.Run("initial state", func(t *testing.T) {
		assert.Equal(t, 0, b.Used())
		assert.Equal(t, 184000, b.Available())
		assert.InDelta(t, 0.0, b.UsagePercent(), 0.01)
	})

	t.Run("with usage", func(t *testing.T) {
		b.SystemPrompt = 5000
		b.Tools = 3000
		b.Messages = 50000
		assert.Equal(t, 58000, b.Used())
		assert.Equal(t, 126000, b.Available())
	})

	t.Run("needs compact", func(t *testing.T) {
		b.Messages = 170000
		assert.True(t, b.NeedsCompact())
	})

	t.Run("needs warning", func(t *testing.T) {
		b.Messages = 160000
		assert.True(t, b.NeedsWarning())
	})

	t.Run("available floor at zero", func(t *testing.T) {
		b.Messages = 200000
		assert.Equal(t, 0, b.Available())
	})
}

func TestBudget_Update(t *testing.T) {
	c := NewTokenCounter()
	b := NewBudget(200000, 16000)

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("Hello")}},
	}
	tools := []llm.ToolDefinition{
		{Name: "Read", Description: "Read a file", InputSchema: []byte(`{"type":"object"}`)},
	}

	b.Update(c, "You are an AI assistant.", tools, messages)
	assert.Greater(t, b.SystemPrompt, 0)
	assert.Greater(t, b.Tools, 0)
	assert.Greater(t, b.Messages, 0)
}

func TestFormatBudgetBar(t *testing.T) {
	b := NewBudget(200000, 16000)
	b.Messages = 92000 // ~50%

	bar := FormatBudgetBar(b)
	require.Contains(t, bar, "[")
	require.Contains(t, bar, "]")
	assert.Contains(t, bar, "█")
	assert.Contains(t, bar, "░")
}
