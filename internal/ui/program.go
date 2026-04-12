package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// NewProgram creates a bubbletea program for the REPL UI.
// It injects the program reference into the model's shared programRef
// so that streaming callbacks can send events to the UI.
func NewProgram(m Model) *tea.Program {
	p := tea.NewProgram(m, tea.WithAltScreen())
	// Since bubbletea copies the Model, we use the shared programRef pointer
	// which both the original and the copy point to.
	if m.programRef != nil {
		m.programRef.p = p
	}
	return p
}
