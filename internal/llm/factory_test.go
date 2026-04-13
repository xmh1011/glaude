package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewProvider_Anthropic(t *testing.T) {
	p, err := NewProvider(context.Background(), "anthropic")
	assert.NoError(t, err)
	// Should be a RetryProvider wrapping AnthropicProvider
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok, "should be RetryProvider")
	_, ok = retry.inner.(*AnthropicProvider)
	assert.True(t, ok, "inner should be AnthropicProvider")
}

func TestNewProvider_Default(t *testing.T) {
	p, err := NewProvider(context.Background(), "")
	assert.NoError(t, err)
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok)
	_, ok = retry.inner.(*AnthropicProvider)
	assert.True(t, ok, "empty provider name should default to Anthropic")
}

func TestNewProvider_OpenAI(t *testing.T) {
	p, err := NewProvider(context.Background(), "openai")
	assert.NoError(t, err)
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok)
	// Should be DialectFixer wrapping OpenAIProvider
	fixer, ok := retry.inner.(*DialectFixer)
	assert.True(t, ok, "non-anthropic should have DialectFixer")
	_, ok = fixer.inner.(*OpenAIProvider)
	assert.True(t, ok, "inner should be OpenAIProvider")
}

func TestNewProvider_Ollama(t *testing.T) {
	p, err := NewProvider(context.Background(), "ollama")
	assert.NoError(t, err)
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok)
	fixer, ok := retry.inner.(*DialectFixer)
	assert.True(t, ok, "ollama should have DialectFixer")
	_, ok = fixer.inner.(*OpenAIProvider)
	assert.True(t, ok, "ollama uses OpenAIProvider internally")
}

func TestNewProvider_DeepSeek(t *testing.T) {
	p, err := NewProvider(context.Background(), "deepseek")
	assert.NoError(t, err)
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok)
	fixer, ok := retry.inner.(*DialectFixer)
	assert.True(t, ok, "deepseek should have DialectFixer")
	_, ok = fixer.inner.(*OpenAIProvider)
	assert.True(t, ok, "deepseek uses OpenAIProvider internally")
}

func TestNewProvider_Qwen(t *testing.T) {
	p, err := NewProvider(context.Background(), "qwen")
	assert.NoError(t, err)
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok)
	fixer, ok := retry.inner.(*DialectFixer)
	assert.True(t, ok, "qwen should have DialectFixer")
	_, ok = fixer.inner.(*OpenAIProvider)
	assert.True(t, ok, "qwen uses OpenAIProvider internally")
}

func TestNewProvider_Doubao(t *testing.T) {
	p, err := NewProvider(context.Background(), "doubao")
	assert.NoError(t, err)
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok)
	fixer, ok := retry.inner.(*DialectFixer)
	assert.True(t, ok, "doubao should have DialectFixer")
	_, ok = fixer.inner.(*OpenAIProvider)
	assert.True(t, ok, "doubao uses OpenAIProvider internally")
}

func TestNewProvider_Gemini(t *testing.T) {
	p, err := NewProvider(context.Background(), "gemini")
	assert.NoError(t, err)
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok)
	fixer, ok := retry.inner.(*DialectFixer)
	assert.True(t, ok, "gemini should have DialectFixer")
	_, ok = fixer.inner.(*GeminiProvider)
	assert.True(t, ok, "inner should be GeminiProvider")
}

func TestNewProvider_Qianfan(t *testing.T) {
	p, err := NewProvider(context.Background(), "qianfan")
	assert.NoError(t, err)
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok)
	fixer, ok := retry.inner.(*DialectFixer)
	assert.True(t, ok, "qianfan should have DialectFixer")
	_, ok = fixer.inner.(*OpenAIProvider)
	assert.True(t, ok, "qianfan uses OpenAIProvider internally")
}

func TestNewProvider_OneAPI(t *testing.T) {
	p, err := NewProvider(context.Background(), "oneapi")
	assert.NoError(t, err)
	retry, ok := p.(*RetryProvider)
	assert.True(t, ok)
	fixer, ok := retry.inner.(*DialectFixer)
	assert.True(t, ok, "oneapi should have DialectFixer")
	_, ok = fixer.inner.(*OpenAIProvider)
	assert.True(t, ok, "oneapi is an alias for qianfan, uses OpenAIProvider internally")
}
