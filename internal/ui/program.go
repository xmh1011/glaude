package ui

import tea "github.com/charmbracelet/bubbletea"

// NewProgram creates a bubbletea program for the REPL UI.
func NewProgram(m Model) *tea.Program {
	return tea.NewProgram(m, tea.WithAltScreen())
}
