package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// NewProgram creates a bubbletea program for the REPL UI.
// It stores the program reference directly on the model pointer
// so that streaming callbacks can send events to the UI.
func NewProgram(m *Model) *tea.Program {
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.program = p
	return p
}
