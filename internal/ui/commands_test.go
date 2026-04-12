package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"glaude/internal/agent"
	"glaude/internal/llm"
	"glaude/internal/memory"
)

func TestHandleSlashCommand_Help(t *testing.T) {
	m := newTestModel(t)
	m, _ = m.handleSlashCommand("/help")
	require.NotEmpty(t, m.messages)

	last := m.messages[len(m.messages)-1]
	assert.Equal(t, llm.RoleAssistant, last.role)
	assert.Contains(t, last.text, "Available Commands")
	assert.Contains(t, last.text, "/undo")
	assert.Contains(t, last.text, "/clear")
	assert.Contains(t, last.text, "/exit")
}

func TestHandleSlashCommand_Clear(t *testing.T) {
	m := newTestModel(t)
	m.messages = append(m.messages, displayMessage{role: llm.RoleUser, text: "hello"})
	m.messages = append(m.messages, displayMessage{role: llm.RoleAssistant, text: "hi"})

	m, _ = m.handleSlashCommand("/clear")
	require.Len(t, m.messages, 1)
	assert.Contains(t, m.messages[0].text, "cleared")
}

func TestHandleSlashCommand_Context(t *testing.T) {
	m := newTestModel(t)
	m, _ = m.handleSlashCommand("/context")
	require.NotEmpty(t, m.messages)

	last := m.messages[len(m.messages)-1]
	assert.Contains(t, last.text, "Context Info")
	assert.Contains(t, last.text, "Messages in conversation")
}

func TestHandleSlashCommand_Undo_NoCheckpoint(t *testing.T) {
	m := newTestModel(t)
	m.checkpoint = nil
	m, _ = m.handleSlashCommand("/undo")
	require.NotEmpty(t, m.messages)

	last := m.messages[len(m.messages)-1]
	assert.Contains(t, last.text, "No checkpoint")
}

func TestHandleSlashCommand_Undo_EmptyStack(t *testing.T) {
	m := newTestModel(t)
	m, _ = m.handleSlashCommand("/undo")
	require.NotEmpty(t, m.messages)

	last := m.messages[len(m.messages)-1]
	assert.Contains(t, last.text, "nothing to undo")
}

func TestHandleSlashCommand_Undo_WithCheckpoint(t *testing.T) {
	m := newTestModel(t)
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	os.WriteFile(path, []byte("original"), 0644)

	require.NoError(t, m.checkpoint.Save("tx-1", path))
	os.WriteFile(path, []byte("modified"), 0644)

	m, _ = m.handleSlashCommand("/undo")
	require.NotEmpty(t, m.messages)

	last := m.messages[len(m.messages)-1]
	assert.Contains(t, last.text, "Undone transaction: tx-1")

	// Verify file was restored
	data, _ := os.ReadFile(path)
	assert.Equal(t, "original", string(data))
}

func TestHandleSlashCommand_Unknown(t *testing.T) {
	m := newTestModel(t)
	m, _ = m.handleSlashCommand("/unknown_xyz")
	require.NotEmpty(t, m.messages)

	last := m.messages[len(m.messages)-1]
	assert.Contains(t, last.text, "Unknown command")
	assert.Contains(t, last.text, "/help")
}

func TestHandleSlashCommand_Exit(t *testing.T) {
	m := newTestModel(t)
	m, cmd := m.handleSlashCommand("/exit")
	assert.True(t, m.quitting)
	assert.NotNil(t, cmd)
}

func TestHandleSlashCommand_Quit(t *testing.T) {
	m := newTestModel(t)
	m, cmd := m.handleSlashCommand("/quit")
	assert.True(t, m.quitting)
	assert.NotNil(t, cmd)
}

// --- helpers ---

// mockProvider is a minimal LLM provider for testing.
type testMockProvider struct{}

func (p *testMockProvider) Complete(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	return &llm.Response{
		Content:    []llm.ContentBlock{llm.NewTextBlock("mock response")},
		StopReason: llm.StopEndTurn,
	}, nil
}

func newTestModel(t *testing.T) Model {
	t.Helper()
	cp := memory.NewCheckpoint()
	a := agent.New(&testMockProvider{}, "test-model", "test prompt", nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	return Model{
		agent:      a,
		checkpoint: cp,
		ctx:        ctx,
		cancel:     cancel,
	}
}
