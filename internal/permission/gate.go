package permission

import "context"

// PromptFunc is a callback that asks the user for permission.
// It receives context, the tool name, a description of the operation,
// and the scan result (if applicable). It returns true if the user approves.
type PromptFunc func(ctx context.Context, toolName string, description string, scan *ScanResult) bool

// Gate combines a Checker with a PromptFunc to make permission decisions.
// When the Checker returns Ask, the Gate calls the PromptFunc.
// When no PromptFunc is set, Ask is treated as Deny (headless mode).
type Gate struct {
	checker    *Checker
	promptFunc PromptFunc
}

// NewGate creates a Gate with the given checker and optional prompt function.
func NewGate(checker *Checker, prompt PromptFunc) *Gate {
	return &Gate{checker: checker, promptFunc: prompt}
}

// Evaluate runs the full permission pipeline for a tool invocation:
// 1. Check mode-based permission
// 2. For Bash, run the danger scanner
// 3. If Ask, invoke the prompt callback
// Returns the final allow/deny decision and reason.
func (g *Gate) Evaluate(ctx context.Context, toolName string, isReadOnly bool, bashCmd string) CheckResult {
	// Step 1: Mode-based check
	result := g.checker.Check(toolName, isReadOnly, bashCmd)

	// Step 2: For Bash commands, run danger scanner regardless of mode decision.
	// If scanner finds high-severity threats and mode would auto-allow, escalate to Ask.
	var scan *ScanResult
	if toolName == "Bash" && bashCmd != "" {
		s := ScanCommand(bashCmd)
		scan = &s
		if !s.Safe && s.HasHighSeverity() && result.Decision == Allow {
			result = CheckResult{
				Decision: Ask,
				Reason:   "high-severity threat detected: " + s.Summary(),
				Tool:     toolName,
			}
		}
	}

	// Step 3: Resolve Ask via prompt callback
	if result.Decision == Ask {
		if g.promptFunc == nil {
			// Headless mode: no prompt available → deny
			return CheckResult{
				Decision: Deny,
				Reason:   "no interactive prompt available (headless mode)",
				Tool:     toolName,
			}
		}

		desc := result.Reason
		if scan != nil && !scan.Safe {
			desc = scan.Summary()
		}

		if g.promptFunc(ctx, toolName, desc, scan) {
			return CheckResult{Decision: Allow, Reason: "user approved", Tool: toolName}
		}
		return CheckResult{Decision: Deny, Reason: "user denied", Tool: toolName}
	}

	return result
}

// Checker returns the underlying permission checker.
func (g *Gate) Checker() *Checker {
	return g.checker
}
