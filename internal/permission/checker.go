package permission

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Decision represents the outcome of a permission check.
type Decision int

const (
	Allow  Decision = iota // tool may proceed
	Deny                   // tool is rejected outright
	Ask                    // user must confirm interactively
)

// String returns the decision name.
func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case Deny:
		return "deny"
	case Ask:
		return "ask"
	default:
		return fmt.Sprintf("unknown(%d)", int(d))
	}
}

// CheckResult holds the outcome of a permission evaluation with context.
type CheckResult struct {
	Decision Decision
	Reason   string // human-readable reason for the decision
	Tool     string // tool name that was checked
}

// Checker evaluates whether a tool invocation is permitted under the current mode.
type Checker struct {
	mode Mode
}

// NewChecker creates a Checker from configuration.
// It reads the "permission_mode" setting from viper.
func NewChecker() *Checker {
	modeStr := viper.GetString("permission_mode")
	return &Checker{mode: ParseMode(modeStr)}
}

// NewCheckerWithMode creates a Checker with an explicit mode.
func NewCheckerWithMode(mode Mode) *Checker {
	return &Checker{mode: mode}
}

// Mode returns the current permission mode.
func (c *Checker) Mode() Mode {
	return c.mode
}

// SetMode changes the permission mode at runtime.
func (c *Checker) SetMode(m Mode) {
	c.mode = m
}

// Check evaluates whether the given tool call should be allowed, denied, or
// needs user confirmation.
//
// Parameters:
//   - toolName: the tool's Name() (e.g. "Bash", "Edit", "Read")
//   - isReadOnly: whether the tool reports itself as read-only
//   - bashCmd: for Bash tool, the actual command string (empty for non-Bash)
func (c *Checker) Check(toolName string, isReadOnly bool, bashCmd string) CheckResult {
	// Step 1: Read-only tools are always allowed in all modes.
	if isReadOnly {
		return CheckResult{Decision: Allow, Reason: "read-only tool", Tool: toolName}
	}

	// Step 2: Mode-specific evaluation for mutating tools.
	switch c.mode {
	case ModePlanOnly:
		return CheckResult{
			Decision: Deny,
			Reason:   "plan-only mode: mutations not allowed",
			Tool:     toolName,
		}

	case ModeAutoFull:
		return CheckResult{
			Decision: Allow,
			Reason:   "auto-full mode: all operations allowed",
			Tool:     toolName,
		}

	case ModeAutoEdit:
		if isEditTool(toolName) {
			return CheckResult{
				Decision: Allow,
				Reason:   "auto-edit mode: file edits auto-approved",
				Tool:     toolName,
			}
		}
		// Non-edit mutating tools (Bash) still need approval
		return CheckResult{
			Decision: Ask,
			Reason:   fmt.Sprintf("auto-edit mode: %s requires approval", toolName),
			Tool:     toolName,
		}

	default: // ModeDefault
		return CheckResult{
			Decision: Ask,
			Reason:   fmt.Sprintf("default mode: %s requires approval", toolName),
			Tool:     toolName,
		}
	}
}

// isEditTool returns true for file-editing tools that auto-edit mode allows.
func isEditTool(name string) bool {
	switch strings.ToLower(name) {
	case "edit", "write":
		return true
	default:
		return false
	}
}
