package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/xmh1011/glaude/internal/tool/askuser"
)

// askUserMsg is sent from the Agent goroutine to the UI when the
// AskUserQuestion tool is invoked.
type askUserMsg struct {
	questions  []askuser.Question
	responseCh chan askUserResp
}

// askUserResp carries the user's answers back to the blocked Agent goroutine.
type askUserResp struct {
	answers map[string]string
	ok      bool
}

// WireAskUser connects the AskUserQuestion tool to the bubbletea UI.
// It uses the same channel bridge pattern as WirePermissionGate:
// the Agent goroutine sends a message via p.Send() and blocks until
// the user completes the interaction.
func WireAskUser(t *askuser.Tool, p *tea.Program) {
	t.AnswerFn = func(ctx context.Context, questions []askuser.Question) (map[string]string, bool) {
		responseCh := make(chan askUserResp, 1)

		p.Send(askUserMsg{
			questions:  questions,
			responseCh: responseCh,
		})

		select {
		case <-ctx.Done():
			return nil, false
		case resp := <-responseCh:
			return resp.answers, resp.ok
		}
	}
}

// handleAskUserKey processes key presses during an AskUserQuestion prompt.
func (m *Model) handleAskUserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	req := m.askRequest

	q := req.questions[m.askCurrentQ]

	// "Other" free-text input mode
	if m.askOtherMode {
		switch msg.Type {
		case tea.KeyEsc:
			m.askOtherMode = false
			m.askOtherBuf = ""
			return m, nil
		case tea.KeyEnter:
			text := strings.TrimSpace(m.askOtherBuf)
			if text != "" {
				m.askAnswers[q.Question] = text
				m.askOtherMode = false
				m.askOtherBuf = ""
				return m.advanceAskQuestion()
			}
			return m, nil
		case tea.KeyBackspace:
			if len(m.askOtherBuf) > 0 {
				m.askOtherBuf = m.askOtherBuf[:len(m.askOtherBuf)-1]
			}
			return m, nil
		default:
			if len(msg.Runes) > 0 {
				m.askOtherBuf += string(msg.Runes)
			}
			return m, nil
		}
	}

	switch msg.Type {
	case tea.KeyEsc:
		// Cancel the entire question flow
		req.responseCh <- askUserResp{ok: false}
		m.resetAskState()
		return m, nil

	case tea.KeyEnter:
		if q.MultiSelect {
			// Commit multi-select: gather selected labels
			var selected []string
			for _, idx := range m.askSelected {
				if idx < len(q.Options) {
					selected = append(selected, q.Options[idx].Label)
				}
			}
			if len(selected) == 0 {
				return m, nil // must select at least one
			}
			m.askAnswers[q.Question] = strings.Join(selected, ", ")
		}
		return m.advanceAskQuestion()

	default:
		key := msg.String()

		// "o" or "O" enters Other/custom input mode
		if key == "o" || key == "O" {
			m.askOtherMode = true
			m.askOtherBuf = ""
			return m, nil
		}

		// Number keys select options (1-based)
		if n, err := strconv.Atoi(key); err == nil && n >= 1 && n <= len(q.Options) {
			idx := n - 1
			if q.MultiSelect {
				// Toggle selection
				found := -1
				for i, s := range m.askSelected {
					if s == idx {
						found = i
						break
					}
				}
				if found >= 0 {
					m.askSelected = append(m.askSelected[:found], m.askSelected[found+1:]...)
				} else {
					m.askSelected = append(m.askSelected, idx)
				}
			} else {
				// Single select: immediately commit
				m.askAnswers[q.Question] = q.Options[idx].Label
				return m.advanceAskQuestion()
			}
		}
	}

	return m, nil
}

// advanceAskQuestion moves to the next question or completes the flow.
func (m *Model) advanceAskQuestion() (tea.Model, tea.Cmd) {
	req := m.askRequest
	m.askCurrentQ++
	m.askSelected = nil

	if m.askCurrentQ >= len(req.questions) {
		// All questions answered
		req.responseCh <- askUserResp{
			answers: m.askAnswers,
			ok:      true,
		}
		m.resetAskState()
		return m, nil
	}
	return m, nil
}

// resetAskState clears all ask-related model fields.
func (m *Model) resetAskState() {
	m.askPrompt = false
	m.askRequest = nil
	m.askCurrentQ = 0
	m.askSelected = nil
	m.askAnswers = nil
	m.askOtherMode = false
	m.askOtherBuf = ""
}

// renderAskUser renders the question prompt UI.
func (m *Model) renderAskUser() string {
	req := m.askRequest
	q := req.questions[m.askCurrentQ]

	var b strings.Builder

	// Question header chip
	headerChip := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("62")).
		Padding(0, 1).
		Render(q.Header)

	b.WriteString(headerChip)
	b.WriteString("\n\n")

	// Question text
	qText := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		Render("  " + q.Question)
	b.WriteString(qText)
	b.WriteString("\n\n")

	// Options
	for i, opt := range q.Options {
		num := fmt.Sprintf("  %d", i+1)
		label := opt.Label
		desc := opt.Description

		// Check if selected (multi-select)
		isSelected := false
		for _, s := range m.askSelected {
			if s == i {
				isSelected = true
				break
			}
		}

		if isSelected {
			marker := lipgloss.NewStyle().Foreground(successColor).Bold(true).Render("[x]")
			nameStr := lipgloss.NewStyle().Foreground(accentColor).Bold(true).Render(label)
			b.WriteString(fmt.Sprintf("  %s %s %s", num, marker, nameStr))
		} else {
			marker := lipgloss.NewStyle().Foreground(dimColor).Render("[ ]")
			nameStr := lipgloss.NewStyle().Foreground(accentColor).Bold(true).Render(label)
			if q.MultiSelect {
				b.WriteString(fmt.Sprintf("  %s %s %s", num, marker, nameStr))
			} else {
				b.WriteString(fmt.Sprintf("  %s. %s", num, nameStr))
			}
		}
		if desc != "" {
			descStr := lipgloss.NewStyle().Foreground(dimColor).Render(" — " + desc)
			b.WriteString(descStr)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// "Other" input mode
	if m.askOtherMode {
		otherLabel := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render("  Other: ")
		cursor := lipgloss.NewStyle().Foreground(accentColor).Render("▌")
		b.WriteString(otherLabel + m.askOtherBuf + cursor + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render("  Enter to confirm, Esc to cancel"))
		b.WriteString("\n")
	} else {
		// Navigation hints
		hint := "  Press 1-%d to select"
		if q.MultiSelect {
			hint = "  Press 1-%d to toggle, Enter to confirm"
		}
		hintStr := fmt.Sprintf(hint, len(q.Options))
		hintStr += " | o: Other | Esc: cancel"

		// Progress indicator
		if len(req.questions) > 1 {
			hintStr += fmt.Sprintf(" | (%d/%d)", m.askCurrentQ+1, len(req.questions))
		}

		b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(hintStr))
		b.WriteString("\n")
	}

	return b.String()
}
