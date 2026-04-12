package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewProvider_Anthropic(t *testing.T) {
	p := NewProvider("anthropic", "claude-sonnet-4-20250514")
	// Should be a RetryProvider wrapping AnthropicProvider
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok, "should be RetryProvider")
	_, ok = retry.inner.(*AnthropicProvider)
	assert.True(t, ok, "inner should be AnthropicProvider")
}

func TestNewProvider_Default(t *testing.T) {
	p := NewProvider("", "claude-sonnet-4-20250514")
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok)
	_, ok = retry.inner.(*AnthropicProvider)
	assert.True(t, ok, "empty provider name should default to Anthropic")
}

func TestNewProvider_OpenAI(t *testing.T) {
	p := NewProvider("openai", "gpt-4")
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok)
	// Should be DialectFixer wrapping OpenAIProvider
	fixer, ok := retry.inner.(*DialectFixer)
	assert.True(t, ok, "non-anthropic should have DialectFixer")
	_, ok = fixer.inner.(*OpenAIProvider)
	assert.True(t, ok, "inner should be OpenAIProvider")
}

func TestNewProvider_Ollama(t *testing.T) {
	p := NewProvider("ollama", "qwen2.5:7b")
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok)
	fixer, ok := retry.inner.(*DialectFixer)
	assert.True(t, ok, "ollama should have DialectFixer")
	_, ok = fixer.inner.(*OpenAIProvider)
	assert.True(t, ok, "ollama uses OpenAIProvider internally")
}
