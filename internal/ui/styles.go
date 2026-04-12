package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Color palette.
var (
	primaryColor = lipgloss.Color("99")  // purple
	accentColor  = lipgloss.Color("205") // pink
	successColor = lipgloss.Color("82")  // green
	errorColor   = lipgloss.Color("196") // red
	dimColor     = lipgloss.Color("240") // gray
	addColor     = lipgloss.Color("34")  // diff green
	delColor     = lipgloss.Color("160") // diff red
)

// Component styles.
var (
	headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(primaryColor).
		Padding(0, 1)

	userLabelStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12"))

	assistantLabelStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor)

	userMsgStyle = lipgloss.NewStyle().
		PaddingLeft(2)

	assistantMsgStyle = lipgloss.NewStyle().
		PaddingLeft(2)

	spinnerStyle = lipgloss.NewStyle().
		Foreground(accentColor).
		PaddingLeft(2)

	statusBarStyle = lipgloss.NewStyle().
		Foreground(dimColor).
		Background(lipgloss.Color("236"))

	toolLabelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true)

	toolOutputStyle = lipgloss.NewStyle().
		Foreground(dimColor).
		PaddingLeft(4)

	errorStyle = lipgloss.NewStyle().
		Foreground(errorColor).
		Bold(true)

	successStyle = lipgloss.NewStyle().
		Foreground(successColor)

	// Diff styles
	diffAddStyle = lipgloss.NewStyle().
		Foreground(addColor)

	diffDelStyle = lipgloss.NewStyle().
		Foreground(delColor)

	diffHeaderStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("33")).
		Bold(true)

	diffHunkStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("36"))

	dividerStyle = lipgloss.NewStyle().
		Foreground(dimColor)
)
