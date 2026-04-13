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
	"github.com/xmh1011/glaude/internal/skill"
)

// displayMessage is a rendered message for display in the UI.
type displayMessage struct {
	role    llm.Role
	text    string // raw text
	toolUse string // tool name if this is a tool use summary
}

// agentDoneMsg signals that the agent has finished processing.
// Kept for backwards compatibility with non-streaming code paths.
type agentDoneMsg struct {
	text string
	err  error
}

// streamTextMsg delivers a text delta for real-time rendering.
type streamTextMsg struct {
	text string
}

// streamToolStartMsg signals that a tool call is starting.
type streamToolStartMsg struct {
	toolName string
	toolID   string
}

// streamDoneMsg signals the stream has completed for this turn.
type streamDoneMsg struct {
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

	// Streaming state
	streaming  bool   // true while receiving stream events
	streamText string // accumulated text from stream deltas

	// Permission prompt state
	permPrompt  bool                  // true when showing permission prompt
	permRequest *permissionRequestMsg // current permission request

	// Program reference for streaming callbacks.
	// Since Model is used as a pointer, this can be a direct *tea.Program.
	program *tea.Program

	// Context for cancelling agent work
	ctx    context.Context
	cancel context.CancelFunc

	// Skill registry for slash command fallback
	skillRegistry *skill.Registry

	// Slash command completion state
	completions   []completion // filtered candidates
	completionIdx int          // selected index (-1 = none)
}

// completion represents a slash command candidate.
type completion struct {
	name string
	desc string
}

// NewModel creates a new REPL model.
// IMPORTANT: Avoid terminal-querying calls (glamour WithAutoStyle, term.GetSize,
// textarea.Focus) before tea.Program.Run() — they can leave stale data in stdin
// that corrupts bubbletea's input parser, causing intermittent input freezes.
func NewModel(a *agent.Agent, cp *memory.Checkpoint, ctx context.Context) *Model {
	// Text input area — clean prompt style (❯) without borders
	// NOTE: Focus() is deferred to Init() to avoid producing a tea.Cmd before
	// the program is running. Calling it here discards the returned Cmd and
	// can leave the cursor blink state inconsistent.
	ta := textarea.New()
	ta.Placeholder = "Type your message..."
	ta.CharLimit = 0 // unlimited
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.Prompt = "❯ "
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(dimColor)
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(dimColor)
	ta.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(dimColor)

	// Spinner for waiting state
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Markdown renderer — use dark style explicitly to avoid querying the
	// terminal for background color before bubbletea enters raw mode.
	// glamour.WithAutoStyle() sends an OSC 11 query that can leave stale
	// response data in stdin, causing the ANSI input parser to hang.
	r, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(80),
	)

	childCtx, cancel := context.WithCancel(ctx)

	return &Model{
		agent:      a,
		checkpoint: cp,
		textarea:   ta,
		spinner:    sp,
		renderer:   r,
		width:      80, // default; updated by WindowSizeMsg from bubbletea
		ctx:        childCtx,
		cancel:     cancel,
	}
}

// SetProgram sets the tea.Program reference needed for streaming callbacks.
// Must be called after NewProgram creates the program.
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

// updateCompletions recomputes the completion list based on current input.
func (m *Model) updateCompletions() {
	input := m.textarea.Value()
	if !strings.HasPrefix(input, "/") {
		m.completions = nil
		m.completionIdx = -1
		return
	}

	// Extract the partial command name after /
	prefix := strings.ToLower(strings.TrimPrefix(input, "/"))
	prefix = strings.SplitN(prefix, " ", 2)[0] // only match command name part

	var candidates []completion

	// Built-in commands
	for _, cmd := range commands() {
		if strings.HasPrefix(cmd.name, prefix) {
			candidates = append(candidates, completion{name: cmd.name, desc: cmd.description})
		}
	}

	// Skills
	if m.skillRegistry != nil {
		for _, s := range m.skillRegistry.UserInvocable() {
			if strings.HasPrefix(s.Name, prefix) {
				desc := s.Description
				if desc == "" {
					desc = "Skill"
				}
				candidates = append(candidates, completion{name: s.Name, desc: desc})
			}
		}
	}

	m.completions = candidates
	if len(candidates) > 0 && m.completionIdx >= len(candidates) {
		m.completionIdx = 0
	}
	if len(candidates) == 0 {
		m.completionIdx = -1
	} else if m.completionIdx < 0 {
		m.completionIdx = 0
	}
}

// SetSkillRegistry sets the skill registry for slash command fallback.
// When a slash command is not a built-in command, it falls back to looking up
// skills in this registry.
func (m *Model) SetSkillRegistry(reg *skill.Registry) {
	m.skillRegistry = reg
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.textarea.Focus(),
		m.spinner.Tick,
	)
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Permission prompt intercepts all key input
		if m.permPrompt && m.permRequest != nil {
			return m.handlePermissionKey(msg)
		}

		// Completion navigation when candidates are visible
		if len(m.completions) > 0 && !m.waiting {
			switch msg.Type {
			case tea.KeyUp:
				if m.completionIdx > 0 {
					m.completionIdx--
				} else {
					m.completionIdx = len(m.completions) - 1
				}
				return m, nil
			case tea.KeyDown:
				if m.completionIdx < len(m.completions)-1 {
					m.completionIdx++
				} else {
					m.completionIdx = 0
				}
				return m, nil
			case tea.KeyTab:
				// Accept selected completion
				sel := m.completions[m.completionIdx]
				m.textarea.SetValue("/" + sel.name + " ")
				m.textarea.CursorEnd()
				m.completions = nil
				m.completionIdx = -1
				return m, nil
			case tea.KeyEsc:
				m.completions = nil
				m.completionIdx = -1
				return m, nil
			}
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
				m.streaming = false
				m.streamText = ""
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
			m.completions = nil
			m.completionIdx = -1

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
			m.streaming = true
			m.streamText = ""
			m.err = nil

			return m, m.runAgent(input)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width)
		if m.renderer != nil {
			m.renderer, _ = glamour.NewTermRenderer(
				glamour.WithStandardStyle("dark"),
				glamour.WithWordWrap(msg.Width-4),
			)
		}

	case permissionRequestMsg:
		m.permPrompt = true
		m.permRequest = &msg
		return m, nil

	case streamTextMsg:
		m.streamText += msg.text
		return m, nil

	case streamToolStartMsg:
		// Finalize any accumulated stream text as a message
		if m.streamText != "" {
			m.messages = append(m.messages, displayMessage{
				role: llm.RoleAssistant,
				text: m.streamText,
			})
			m.streamText = ""
		}
		m.streaming = false // switch to spinner during tool execution
		m.messages = append(m.messages, displayMessage{
			role:    llm.RoleAssistant,
			toolUse: msg.toolName,
			text:    fmt.Sprintf("Using tool: **%s**", msg.toolName),
		})
		return m, nil

	case streamDoneMsg:
		m.waiting = false
		m.streaming = false
		m.permPrompt = false
		m.permRequest = nil
		if msg.err != nil {
			m.err = msg.err
			// If there was partial stream text, include it
			if m.streamText != "" {
				m.messages = append(m.messages, displayMessage{
					role: llm.RoleAssistant,
					text: m.streamText,
				})
				m.streamText = ""
			}
			m.messages = append(m.messages, displayMessage{
				role: llm.RoleAssistant,
				text: fmt.Sprintf("Error: %v", msg.err),
			})
		} else if msg.text != "" {
			// The final text supersedes any streamed text
			m.streamText = ""
			m.messages = append(m.messages, displayMessage{
				role: llm.RoleAssistant,
				text: msg.text,
			})
		} else if m.streamText != "" {
			// No final text but we accumulated stream deltas
			m.messages = append(m.messages, displayMessage{
				role: llm.RoleAssistant,
				text: m.streamText,
			})
			m.streamText = ""
		}
		return m, nil

	case agentDoneMsg:
		// Legacy non-streaming path
		m.waiting = false
		m.streaming = false
		m.streamText = ""
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
		// Refresh completions after input changes
		m.updateCompletions()
	}

	return m, tea.Batch(cmds...)
}

// handlePermissionKey processes y/n input during a permission prompt.
func (m *Model) handlePermissionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
func (m *Model) View() string {
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
	} else if m.streaming && m.streamText != "" {
		// Render streaming text with blinking cursor
		label := assistantLabelStyle.Render("Assistant")
		rendered := m.streamText
		if m.renderer != nil {
			if out, err := m.renderer.Render(m.streamText); err == nil {
				rendered = strings.TrimSpace(out)
			}
		}
		cursor := lipgloss.NewStyle().Foreground(accentColor).Render("▌")
		b.WriteString(label + "\n" + assistantMsgStyle.Render(rendered+cursor) + "\n\n")
	} else if m.waiting {
		// Spinner (waiting for first token or tool execution)
		b.WriteString(spinnerStyle.Render(m.spinner.View() + " Thinking..."))
		b.WriteString("\n\n")
	}

	// Input area with dividers and status hint
	if !m.waiting && !m.permPrompt {
		divider := dividerStyle.Render(strings.Repeat("─", max(m.width, 40)))
		b.WriteString(divider + "\n")
		b.WriteString(m.textarea.View())
		b.WriteString("\n")
		// Completion menu
		if len(m.completions) > 0 {
			b.WriteString(m.renderCompletions())
		}
		b.WriteString(divider + "\n")
		b.WriteString(m.renderStatusBar())
	}

	return b.String()
}

// renderPermissionPrompt renders the permission confirmation dialog.
func (m *Model) renderPermissionPrompt() string {
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
func (m *Model) renderMessage(msg displayMessage) string {
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

// renderStatusBar renders a subtle single-line status hint below the input,
// matching Claude Code's minimal bottom-bar style.
func (m *Model) renderStatusBar() string {
	usage := m.agent.TotalUsage()
	budget := m.agent.Budget()

	// Permission mode indicator
	modeStr := "default"
	if m.agent.Gate() != nil {
		modeStr = m.agent.Gate().Checker().Mode().String()
	}

	pct := budget.UsagePercent()
	ctx := fmt.Sprintf("%.0f%%", pct)

	tokens := fmt.Sprintf("%d↑ %d↓", usage.InputTokens, usage.OutputTokens)

	line := fmt.Sprintf(" ►► %s mode | ctx %s | %s", modeStr, ctx, tokens)

	// Pick style based on budget pressure
	style := statusBarStyle
	if budget.NeedsCompact() {
		style = statusBarCritStyle
	} else if budget.NeedsWarning() {
		style = statusBarWarnStyle
	}

	return style.Render(line)
}

// renderCompletions renders the slash command completion menu.
func (m *Model) renderCompletions() string {
	var b strings.Builder
	maxItems := 8
	if len(m.completions) < maxItems {
		maxItems = len(m.completions)
	}

	// Show a window of items around the selected index
	start := 0
	if m.completionIdx >= maxItems {
		start = m.completionIdx - maxItems + 1
	}
	end := start + maxItems
	if end > len(m.completions) {
		end = len(m.completions)
		start = end - maxItems
		if start < 0 {
			start = 0
		}
	}

	for i := start; i < end; i++ {
		c := m.completions[i]
		name := "/" + c.name
		desc := c.desc

		// Truncate description to fit
		maxDesc := m.width - len(name) - 6
		if maxDesc < 0 {
			maxDesc = 0
		}
		if len(desc) > maxDesc {
			if maxDesc > 3 {
				desc = desc[:maxDesc-1] + "…"
			} else {
				desc = ""
			}
		}

		if i == m.completionIdx {
			line := fmt.Sprintf("  %-20s %s", name, desc)
			b.WriteString(completionSelectedStyle.Render(line))
		} else {
			nameStr := completionNameStyle.Render(fmt.Sprintf("  %-20s", name))
			descStr := completionDescStyle.Render(" " + desc)
			b.WriteString(nameStr + descStr)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// runAgent sends the prompt to the agent in a goroutine and returns a Cmd.
// Uses streaming when a *tea.Program reference is available for real-time updates.
func (m *Model) runAgent(prompt string) tea.Cmd {
	ctx := m.ctx
	a := m.agent
	p := m.program
	return func() tea.Msg {
		if p != nil {
			// Streaming mode: deliver text deltas via p.Send()
			cb := func(event llm.StreamEvent) {
				switch event.Type {
				case llm.EventTextDelta:
					p.Send(streamTextMsg{text: event.Text})
				case llm.EventToolUseStart:
					p.Send(streamToolStartMsg{toolName: event.Name, toolID: event.ID})
				}
			}
			text, err := a.RunStream(ctx, prompt, cb)
			return streamDoneMsg{text: text, err: err}
		}
		// Fallback: synchronous mode (no program reference)
		text, err := a.Run(ctx, prompt)
		return agentDoneMsg{text: text, err: err}
	}
}
