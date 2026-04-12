// Package ui implements the terminal user interface using bubbletea MVU architecture.
//
// The UI follows the Model-View-Update pattern:
//   - Model: holds all application state (messages, input, agent, spinner)
//   - Update: handles keyboard events, agent completion, tool execution events
//   - View: renders message history, input area, and status bar
//
// Permission prompts use a channel-based bridge: the Agent's permission Gate
// sends a request via tea.Program.Send(), the UI renders a [y/n] prompt,
// and the user's response is sent back through a response channel.
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

	"github.com/xmh1011/glaude/internal/agent"
	"github.com/xmh1011/glaude/internal/llm"
	"github.com/xmh1011/glaude/internal/memory"
	"github.com/xmh1011/glaude/internal/permission"
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

// permissionRequestMsg is sent from the Agent's permission Gate to the UI
// when a tool call requires user confirmation.
type permissionRequestMsg struct {
	toolName    string
	description string
	scan        *permission.ScanResult
	responseCh  chan bool
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
	waiting  bool  // true while agent is processing
	err      error // last error
	width    int   // terminal width
	height   int   // terminal height
	quitting bool

	// Permission prompt state
	permPrompt   bool              // true when showing permission prompt
	permRequest  *permissionRequestMsg // current permission request

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
		// Permission prompt intercepts all key input
		if m.permPrompt && m.permRequest != nil {
			return m.handlePermissionKey(msg)
		}

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
				m.permPrompt = false
				m.permRequest = nil
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

	case permissionRequestMsg:
		m.permPrompt = true
		m.permRequest = &msg
		return m, nil

	case agentDoneMsg:
		m.waiting = false
		m.permPrompt = false
		m.permRequest = nil
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
	if !m.waiting && !m.permPrompt {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// handlePermissionKey processes y/n input during a permission prompt.
func (m Model) handlePermissionKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.messages = append(m.messages, displayMessage{
			role: llm.RoleAssistant,
			text: fmt.Sprintf("Approved: %s", m.permRequest.toolName),
		})
		m.permRequest.responseCh <- true
		m.permPrompt = false
		m.permRequest = nil
		return m, nil

	case "n", "N":
		m.messages = append(m.messages, displayMessage{
			role: llm.RoleAssistant,
			text: fmt.Sprintf("Denied: %s", m.permRequest.toolName),
		})
		m.permRequest.responseCh <- false
		m.permPrompt = false
		m.permRequest = nil
		return m, nil
	}
	// Ignore other keys while in permission prompt
	return m, nil
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

	// Permission prompt
	if m.permPrompt && m.permRequest != nil {
		b.WriteString(m.renderPermissionPrompt())
		b.WriteString("\n")
	} else if m.waiting {
		// Spinner
		b.WriteString(spinnerStyle.Render(m.spinner.View() + " Thinking..."))
		b.WriteString("\n\n")
	}

	// Input area
	if !m.waiting && !m.permPrompt {
		b.WriteString(m.textarea.View())
		b.WriteString("\n")
	}

	// Status bar
	b.WriteString(m.renderStatusBar())

	return b.String()
}

// renderPermissionPrompt renders the permission confirmation dialog.
func (m Model) renderPermissionPrompt() string {
	req := m.permRequest
	var b strings.Builder

	// Warning header
	warning := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		Render("⚠ Permission Required")
	b.WriteString(warning)
	b.WriteString("\n\n")

	// Tool info
	b.WriteString(fmt.Sprintf("  Tool: %s\n", toolLabelStyle.Render(req.toolName)))
	b.WriteString(fmt.Sprintf("  Reason: %s\n", req.description))

	// Threat details
	if req.scan != nil && !req.scan.Safe {
		b.WriteString("\n")
		threatHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("208")).
			Render("  Detected threats:")
		b.WriteString(threatHeader)
		b.WriteString("\n")
		for _, t := range req.scan.Threats {
			severity := t.Severity
			switch severity {
			case "high":
				severity = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("HIGH")
			case "medium":
				severity = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render("MED")
			default:
				severity = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("LOW")
			}
			b.WriteString(fmt.Sprintf("    [%s] %s (%s)\n", severity, t.Description, t.Category))
		}
	}

	b.WriteString("\n")
	prompt := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("82")).
		Render("  Allow this operation? [y/n]")
	b.WriteString(prompt)

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

// renderStatusBar renders the bottom status bar with token budget indicator.
func (m Model) renderStatusBar() string {
	usage := m.agent.TotalUsage()
	budget := m.agent.Budget()

	modeStr := ""
	if m.agent.Gate() != nil {
		modeStr = m.agent.Gate().Checker().Mode().String() + " | "
	}

	left := fmt.Sprintf(" %s%d msgs", modeStr, len(m.messages))

	pct := budget.UsagePercent()
	right := fmt.Sprintf("ctx: %.0f%% | %d in / %d out ",
		pct, usage.InputTokens, usage.OutputTokens)

	gap := ""
	if m.width > len(left)+len(right) {
		gap = strings.Repeat(" ", m.width-len(left)-len(right))
	}

	style := statusBarStyle
	if budget.NeedsCompact() {
		style = statusBarStyle.Background(lipgloss.Color("124")) // red bg
	} else if budget.NeedsWarning() {
		style = statusBarStyle.Background(lipgloss.Color("130")) // yellow bg
	}

	return style.Render(left + gap + right)
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
