// Package ui implements the terminal user interface using bubbletea MVU architecture.
//
// The UI follows the Model-View-Update pattern:
//   - Model: holds all application state (messages, input, agent, spinner)
//   - Update: handles keyboard events, agent completion, tool execution events
//   - View: renders message history, input area, and status bar
package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"glaude/internal/agent"
	"glaude/internal/llm"
	"glaude/internal/memory"
)

// displayMessage is a rendered message for display in the UI.
type displayMessage struct {
	role    llm.Role
	text    string // raw text
	toolUse string // tool name if this is a tool use summary
}

// agentDoneMsg signals that the agent has finished processing.
type agentDoneMsg struct {
	text string
	err  error
}

// Model is the bubbletea model for the REPL interface.
type Model struct {
	agent      *agent.Agent
	checkpoint *memory.Checkpoint

	// UI components
	textarea textarea.Model
	spinner  spinner.Model
	renderer *glamour.TermRenderer

	// State
	messages []displayMessage
	waiting  bool   // true while agent is processing
	err      error  // last error
	width    int    // terminal width
	height   int    // terminal height
	quitting bool

	// Context for cancelling agent work
	ctx    context.Context
	cancel context.CancelFunc
}

// NewModel creates a new REPL model.
func NewModel(a *agent.Agent, cp *memory.Checkpoint, ctx context.Context) Model {
	// Text input area
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Enter to send, Ctrl+D to exit)"
	ta.Focus()
	ta.CharLimit = 0 // unlimited
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	// Spinner for waiting state
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Markdown renderer
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	childCtx, cancel := context.WithCancel(ctx)

	return Model{
		agent:      a,
		checkpoint: cp,
		textarea:   ta,
		spinner:    sp,
		renderer:   r,
		ctx:        childCtx,
		cancel:     cancel,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
	)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlD:
			m.quitting = true
			m.cancel()
			return m, tea.Quit

		case tea.KeyCtrlC:
			if m.waiting {
				// Cancel current agent work
				m.cancel()
				m.waiting = false
				m.messages = append(m.messages, displayMessage{
					role: llm.RoleAssistant,
					text: "(interrupted)",
				})
				// Create new context for next interaction
				m.ctx, m.cancel = context.WithCancel(context.Background())
				return m, nil
			}
			m.quitting = true
			m.cancel()
			return m, tea.Quit

		case tea.KeyEnter:
			if msg.Alt {
				// Alt+Enter: insert newline
				var cmd tea.Cmd
				m.textarea, cmd = m.textarea.Update(msg)
				return m, cmd
			}

			if m.waiting {
				return m, nil
			}

			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}

			m.textarea.Reset()

			// Check for slash commands
			if strings.HasPrefix(input, "/") {
				return m.handleSlashCommand(input)
			}

			// Send to agent
			m.messages = append(m.messages, displayMessage{
				role: llm.RoleUser,
				text: input,
			})
			m.waiting = true
			m.err = nil

			return m, m.runAgent(input)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width)
		if m.renderer != nil {
			m.renderer, _ = glamour.NewTermRenderer(
				glamour.WithAutoStyle(),
				glamour.WithWordWrap(msg.Width-4),
			)
		}

	case agentDoneMsg:
		m.waiting = false
		if msg.err != nil {
			m.err = msg.err
			m.messages = append(m.messages, displayMessage{
				role: llm.RoleAssistant,
				text: fmt.Sprintf("Error: %v", msg.err),
			})
		} else if msg.text != "" {
			m.messages = append(m.messages, displayMessage{
				role: llm.RoleAssistant,
				text: msg.text,
			})
		}
		return m, nil

	case spinner.TickMsg:
		if m.waiting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Pass remaining events to textarea
	if !m.waiting {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	var b strings.Builder

	// Header
	header := headerStyle.Render(" glaude ")
	b.WriteString(header)
	b.WriteString("\n\n")

	// Messages
	for _, msg := range m.messages {
		b.WriteString(m.renderMessage(msg))
		b.WriteString("\n")
	}

	// Spinner
	if m.waiting {
		b.WriteString(spinnerStyle.Render(m.spinner.View() + " Thinking..."))
		b.WriteString("\n\n")
	}

	// Input area
	if !m.waiting {
		b.WriteString(m.textarea.View())
		b.WriteString("\n")
	}

	// Status bar
	b.WriteString(m.renderStatusBar())

	return b.String()
}

// renderMessage renders a single message with appropriate styling.
func (m Model) renderMessage(msg displayMessage) string {
	switch msg.role {
	case llm.RoleUser:
		label := userLabelStyle.Render("You")
		return label + "\n" + userMsgStyle.Render(msg.text) + "\n"

	case llm.RoleAssistant:
		label := assistantLabelStyle.Render("Assistant")
		rendered := msg.text
		if m.renderer != nil {
			if out, err := m.renderer.Render(msg.text); err == nil {
				rendered = strings.TrimSpace(out)
			}
		}
		return label + "\n" + assistantMsgStyle.Render(rendered) + "\n"

	default:
		return msg.text + "\n"
	}
}

// renderStatusBar renders the bottom status bar.
func (m Model) renderStatusBar() string {
	usage := m.agent.TotalUsage()
	left := fmt.Sprintf(" %d msgs", len(m.messages))
	right := fmt.Sprintf("tokens: %d in / %d out ",
		usage.InputTokens, usage.OutputTokens)

	gap := ""
	if m.width > len(left)+len(right) {
		gap = strings.Repeat(" ", m.width-len(left)-len(right))
	}

	return statusBarStyle.Render(left + gap + right)
}

// runAgent sends the prompt to the agent in a goroutine and returns a Cmd.
func (m Model) runAgent(prompt string) tea.Cmd {
	ctx := m.ctx
	a := m.agent
	return func() tea.Msg {
		text, err := a.Run(ctx, prompt)
		return agentDoneMsg{text: text, err: err}
	}
}
