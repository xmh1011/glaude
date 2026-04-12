package ui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"glaude/internal/agent"
	"glaude/internal/llm"
	"glaude/internal/memory"
)

func TestNewModel(t *testing.T) {
	cp := memory.NewCheckpoint()
	a := agent.New(&testMockProvider{}, "test-model", "test prompt", nil)
	ctx := context.Background()

	m := NewModel(a, cp, ctx)
	assert.NotNil(t, m.agent)
	assert.NotNil(t, m.checkpoint)
	assert.False(t, m.waiting)
	assert.False(t, m.quitting)
	assert.Empty(t, m.messages)
}

func TestModel_View_Quitting(t *testing.T) {
	cp := memory.NewCheckpoint()
	a := agent.New(&testMockProvider{}, "test-model", "test prompt", nil)
	m := NewModel(a, cp, context.Background())
	m.quitting = true
	view := m.View()
	assert.Contains(t, view, "Goodbye")
}

func TestModel_View_WithMessages(t *testing.T) {
	cp := memory.NewCheckpoint()
	a := agent.New(&testMockProvider{}, "test-model", "test prompt", nil)
	m := NewModel(a, cp, context.Background())
	m.messages = append(m.messages, displayMessage{
		role: llm.RoleUser,
		text: "hello world",
	})
	m.messages = append(m.messages, displayMessage{
		role: llm.RoleAssistant,
		text: "hi there",
	})
	view := m.View()
	assert.Contains(t, view, "hello world")
	assert.Contains(t, view, "hi there")
	assert.Contains(t, view, "glaude")
}

func TestModel_RenderStatusBar(t *testing.T) {
	cp := memory.NewCheckpoint()
	a := agent.New(&testMockProvider{}, "test-model", "test prompt", nil)
	m := NewModel(a, cp, context.Background())
	m.width = 80
	bar := m.renderStatusBar()
	assert.Contains(t, bar, "msgs")
	assert.Contains(t, bar, "ctx:")
}

func TestModel_RenderMessage_User(t *testing.T) {
	cp := memory.NewCheckpoint()
	a := agent.New(&testMockProvider{}, "test-model", "test prompt", nil)
	m := NewModel(a, cp, context.Background())
	rendered := m.renderMessage(displayMessage{
		role: llm.RoleUser,
		text: "test input",
	})
	assert.Contains(t, rendered, "You")
	assert.Contains(t, rendered, "test input")
}

func TestModel_RenderMessage_Assistant(t *testing.T) {
	cp := memory.NewCheckpoint()
	a := agent.New(&testMockProvider{}, "test-model", "test prompt", nil)
	m := NewModel(a, cp, context.Background())
	rendered := m.renderMessage(displayMessage{
		role: llm.RoleAssistant,
		text: "test response",
	})
	assert.Contains(t, rendered, "Assistant")
	assert.Contains(t, rendered, "test response")
}
