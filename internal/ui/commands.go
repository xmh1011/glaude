package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/xmh1011/glaude/internal/llm"
)

// slashCommand defines a registered slash command.
type slashCommand struct {
	name        string
	description string
	handler     func(m Model, args string) (Model, tea.Cmd)
}

// commands returns all registered slash commands.
func commands() []slashCommand {
	return []slashCommand{
		{name: "exit", description: "Exit glaude", handler: cmdExit},
		{name: "quit", description: "Exit glaude (alias)", handler: cmdExit},
		{name: "clear", description: "Clear conversation history", handler: cmdClear},
		{name: "undo", description: "Undo the last file change", handler: cmdUndo},
		{name: "context", description: "Show current context info", handler: cmdContext},
		{name: "help", description: "Show available commands", handler: cmdHelp},
	}
}

// handleSlashCommand parses and dispatches a slash command.
func (m Model) handleSlashCommand(input string) (Model, tea.Cmd) {
	parts := strings.SplitN(strings.TrimPrefix(input, "/"), " ", 2)
	name := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	for _, cmd := range commands() {
		if cmd.name == name {
			return cmd.handler(m, args)
		}
	}

	// Unknown command
	m.messages = append(m.messages, displayMessage{
		role: llm.RoleAssistant,
		text: fmt.Sprintf("Unknown command: /%s. Type /help for available commands.", name),
	})
	return m, nil
}

func cmdExit(m Model, _ string) (Model, tea.Cmd) {
	m.quitting = true
	m.cancel()
	return m, tea.Quit
}

func cmdClear(m Model, _ string) (Model, tea.Cmd) {
	m.messages = nil
	m.err = nil
	m.messages = append(m.messages, displayMessage{
		role: llm.RoleAssistant,
		text: "Conversation cleared.",
	})
	return m, nil
}

func cmdUndo(m Model, _ string) (Model, tea.Cmd) {
	if m.checkpoint == nil {
		m.messages = append(m.messages, displayMessage{
			role: llm.RoleAssistant,
			text: "No checkpoint engine available.",
		})
		return m, nil
	}

	txID, err := m.checkpoint.Undo()
	if err != nil {
		m.messages = append(m.messages, displayMessage{
			role: llm.RoleAssistant,
			text: fmt.Sprintf("Undo failed: %v", err),
		})
		return m, nil
	}

	m.messages = append(m.messages, displayMessage{
		role: llm.RoleAssistant,
		text: fmt.Sprintf("Undone transaction: %s", txID),
	})
	return m, nil
}

func cmdContext(m Model, _ string) (Model, tea.Cmd) {
	usage := m.agent.TotalUsage()
	msgCount := len(m.agent.Messages())
	undoCount := 0
	if m.checkpoint != nil {
		undoCount = m.checkpoint.Len()
	}

	info := fmt.Sprintf(`**Context Info**
- Messages in conversation: %d
- Display messages: %d
- Input tokens used: %d
- Output tokens used: %d
- Undo stack depth: %d`,
		msgCount, len(m.messages),
		usage.InputTokens, usage.OutputTokens,
		undoCount)

	m.messages = append(m.messages, displayMessage{
		role: llm.RoleAssistant,
		text: info,
	})
	return m, nil
}

func cmdHelp(m Model, _ string) (Model, tea.Cmd) {
	var b strings.Builder
	b.WriteString("**Available Commands**\n\n")
	for _, cmd := range commands() {
		b.WriteString(fmt.Sprintf("- `/%s` — %s\n", cmd.name, cmd.description))
	}
	b.WriteString("\n**Keyboard Shortcuts**\n\n")
	b.WriteString("- `Enter` — Send message\n")
	b.WriteString("- `Alt+Enter` — Insert newline\n")
	b.WriteString("- `Ctrl+C` — Cancel current operation / Exit\n")
	b.WriteString("- `Ctrl+D` — Exit\n")

	m.messages = append(m.messages, displayMessage{
		role: llm.RoleAssistant,
		text: b.String(),
	})
	return m, nil
}
