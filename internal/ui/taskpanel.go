package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/xmh1011/glaude/internal/state"
)

// Task panel styles.
var (
	taskDoneIcon    = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("✓")
	taskActiveIcon  = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Render("●")
	taskPendingIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("○")

	taskDoneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Strikethrough(true)
	taskActiveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	taskPendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	planBadgeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("208")).
			Padding(0, 1)
)

// renderTaskPanel renders the persistent task/todo panel.
// Returns empty string when there is nothing to display.
func (m *Model) renderTaskPanel() string {
	if m.state == nil {
		return ""
	}

	var b strings.Builder

	// Plan mode badge
	if m.state.InPlanMode() {
		b.WriteString("  ")
		b.WriteString(planBadgeStyle.Render("PLAN MODE"))
		b.WriteString("\n")
	}

	// Tasks
	tasks := m.state.AllTasks()
	if len(tasks) > 0 {
		done, inProg, open := countTasks(tasks)
		header := fmt.Sprintf("  %d tasks (%d done, %d active, %d open)",
			len(tasks), done, inProg, open)
		b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(header))
		b.WriteString("\n")

		for _, t := range tasks {
			b.WriteString(renderTaskLine(t))
			b.WriteString("\n")
		}
		return b.String()
	}

	// Todos (fallback — simpler list)
	todos := m.state.Todos()
	if len(todos) > 0 {
		for _, todo := range todos {
			b.WriteString(renderTodoLine(todo))
			b.WriteString("\n")
		}
		return b.String()
	}

	// Plan mode with no tasks — still show badge
	if m.state.InPlanMode() {
		return b.String()
	}
	return ""
}

func renderTaskLine(t *state.Task) string {
	var icon, text string
	switch t.Status {
	case state.TaskCompleted:
		icon = taskDoneIcon
		text = taskDoneStyle.Render(t.Subject)
	case state.TaskInProgress:
		icon = taskActiveIcon
		text = taskActiveStyle.Render(t.Subject)
	default:
		icon = taskPendingIcon
		text = taskPendingStyle.Render(t.Subject)
	}
	line := fmt.Sprintf("  %s %s", icon, text)
	if t.Owner != "" {
		line += lipgloss.NewStyle().Foreground(dimColor).Render(
			fmt.Sprintf(" (@%s)", t.Owner))
	}
	return line
}

func renderTodoLine(todo state.TodoItem) string {
	var icon, text string
	switch todo.Status {
	case state.TodoCompleted:
		icon = taskDoneIcon
		text = taskDoneStyle.Render(todo.Content)
	case state.TodoInProgress:
		icon = taskActiveIcon
		text = taskActiveStyle.Render(todo.Content)
	default:
		icon = taskPendingIcon
		text = taskPendingStyle.Render(todo.Content)
	}
	return fmt.Sprintf("  %s %s", icon, text)
}

func countTasks(tasks []*state.Task) (done, inProg, open int) {
	for _, t := range tasks {
		switch t.Status {
		case state.TaskCompleted:
			done++
		case state.TaskInProgress:
			inProg++
		default:
			open++
		}
	}
	return
}
