package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/xmh1011/glaude/internal/agent"
	"github.com/xmh1011/glaude/internal/permission"
)

// WirePermissionGate creates a permission Gate that bridges the Agent's
// permission checks with the bubbletea UI's interactive prompt.
//
// When the Agent's tool execution hits an Ask decision, the Gate sends
// a permissionRequestMsg to the tea.Program via p.Send(). The UI renders
// a [y/n] prompt, and the user's response is returned via a channel.
func WirePermissionGate(a *agent.Agent, p *tea.Program, mode permission.Mode) {
	checker := permission.NewCheckerWithMode(mode)

	promptFn := func(ctx context.Context, toolName string, description string, scan *permission.ScanResult) bool {
		responseCh := make(chan bool, 1)

		// Send permission request to the UI event loop
		p.Send(permissionRequestMsg{
			toolName:    toolName,
			description: description,
			scan:        scan,
			responseCh:  responseCh,
		})

		// Block until the user responds or context is cancelled
		select {
		case <-ctx.Done():
			return false
		case approved := <-responseCh:
			return approved
		}
	}

	gate := permission.NewGate(checker, promptFn)
	a.SetGate(gate)
}
