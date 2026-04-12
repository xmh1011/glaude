package ui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"

	"github.com/xmh1011/glaude/internal/agent"
	"github.com/xmh1011/glaude/internal/llm"
	"github.com/xmh1011/glaude/internal/memory"
	"github.com/xmh1011/glaude/internal/permission"
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

func TestModel_PermissionPrompt_View(t *testing.T) {
	cp := memory.NewCheckpoint()
	a := agent.New(&testMockProvider{}, "test-model", "test prompt", nil)
	m := NewModel(a, cp, context.Background())

	scan := permission.ScanCommand("rm -rf /")
	m.permPrompt = true
	m.permRequest = &permissionRequestMsg{
		toolName:    "Bash",
		description: "high-severity threat detected",
		scan:        &scan,
		responseCh:  make(chan bool, 1),
	}
	m.waiting = true

	view := m.View()
	assert.Contains(t, view, "Permission Required")
	assert.Contains(t, view, "Bash")
	assert.Contains(t, view, "[y/n]")
	// Should NOT show spinner when permission prompt is visible
	assert.NotContains(t, view, "Thinking...")
}

func TestModel_PermissionPrompt_Approve(t *testing.T) {
	cp := memory.NewCheckpoint()
	a := agent.New(&testMockProvider{}, "test-model", "test prompt", nil)
	m := NewModel(a, cp, context.Background())

	responseCh := make(chan bool, 1)
	m.permPrompt = true
	m.permRequest = &permissionRequestMsg{
		toolName:    "Bash",
		description: "test",
		responseCh:  responseCh,
	}

	// Simulate pressing 'y'
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	updated := updatedModel.(Model)
	assert.False(t, updated.permPrompt)
	assert.Nil(t, updated.permRequest)

	// Check channel got true
	got := <-responseCh
	assert.True(t, got)
	assert.Contains(t, updated.messages[len(updated.messages)-1].text, "Approved")
}

func TestModel_PermissionPrompt_Deny(t *testing.T) {
	cp := memory.NewCheckpoint()
	a := agent.New(&testMockProvider{}, "test-model", "test prompt", nil)
	m := NewModel(a, cp, context.Background())

	responseCh := make(chan bool, 1)
	m.permPrompt = true
	m.permRequest = &permissionRequestMsg{
		toolName:    "Edit",
		description: "test",
		responseCh:  responseCh,
	}

	// Simulate pressing 'n'
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := updatedModel.(Model)
	assert.False(t, updated.permPrompt)

	got := <-responseCh
	assert.False(t, got)
	assert.Contains(t, updated.messages[len(updated.messages)-1].text, "Denied")
}

func TestModel_PermissionPrompt_IgnoresOtherKeys(t *testing.T) {
	cp := memory.NewCheckpoint()
	a := agent.New(&testMockProvider{}, "test-model", "test prompt", nil)
	m := NewModel(a, cp, context.Background())

	m.permPrompt = true
	m.permRequest = &permissionRequestMsg{
		toolName:   "Bash",
		description: "test",
		responseCh: make(chan bool, 1),
	}

	// Press 'x' — should be ignored
	updatedModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	updated := updatedModel.(Model)
	assert.True(t, updated.permPrompt, "should still be in permission prompt")
}

func TestModel_StatusBar_ShowsMode(t *testing.T) {
	cp := memory.NewCheckpoint()
	a := agent.New(&testMockProvider{}, "test-model", "test prompt", nil)
	gate := permission.NewGate(permission.NewCheckerWithMode(permission.ModeAutoEdit), nil)
	a.SetGate(gate)

	m := NewModel(a, cp, context.Background())
	m.width = 80
	bar := m.renderStatusBar()
	assert.Contains(t, bar, "auto-edit")
}
