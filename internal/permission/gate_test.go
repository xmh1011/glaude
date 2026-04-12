package permission

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGate_ReadOnlyAlwaysAllowed(t *testing.T) {
	g := NewGate(NewCheckerWithMode(ModeDefault), nil)
	result := g.Evaluate(context.Background(), "Read", true, "")
	assert.Equal(t, Allow, result.Decision)
}

func TestGate_DefaultMode_AskWithPromptApprove(t *testing.T) {
	approved := func(_ context.Context, _ string, _ string, _ *ScanResult) bool {
		return true
	}
	g := NewGate(NewCheckerWithMode(ModeDefault), approved)
	result := g.Evaluate(context.Background(), "Bash", false, "ls -la")
	assert.Equal(t, Allow, result.Decision)
	assert.Equal(t, "user approved", result.Reason)
}

func TestGate_DefaultMode_AskWithPromptDeny(t *testing.T) {
	denied := func(_ context.Context, _ string, _ string, _ *ScanResult) bool {
		return false
	}
	g := NewGate(NewCheckerWithMode(ModeDefault), denied)
	result := g.Evaluate(context.Background(), "Bash", false, "ls -la")
	assert.Equal(t, Deny, result.Decision)
	assert.Equal(t, "user denied", result.Reason)
}

func TestGate_HeadlessMode_AskBecomesDeny(t *testing.T) {
	g := NewGate(NewCheckerWithMode(ModeDefault), nil) // no prompt
	result := g.Evaluate(context.Background(), "Edit", false, "")
	assert.Equal(t, Deny, result.Decision)
	assert.Contains(t, result.Reason, "headless")
}

func TestGate_AutoFull_HighSeverityEscalatesToAsk(t *testing.T) {
	var promptCalled bool
	prompt := func(_ context.Context, _ string, desc string, scan *ScanResult) bool {
		promptCalled = true
		assert.NotNil(t, scan)
		assert.False(t, scan.Safe)
		return false // deny
	}
	g := NewGate(NewCheckerWithMode(ModeAutoFull), prompt)
	result := g.Evaluate(context.Background(), "Bash", false, "rm -rf /")
	assert.True(t, promptCalled)
	assert.Equal(t, Deny, result.Decision)
}

func TestGate_AutoFull_SafeCommandAllowed(t *testing.T) {
	g := NewGate(NewCheckerWithMode(ModeAutoFull), nil)
	result := g.Evaluate(context.Background(), "Bash", false, "ls -la")
	assert.Equal(t, Allow, result.Decision)
}

func TestGate_PlanOnly_DeniedWithoutPrompt(t *testing.T) {
	var promptCalled bool
	prompt := func(_ context.Context, _ string, _ string, _ *ScanResult) bool {
		promptCalled = true
		return true
	}
	g := NewGate(NewCheckerWithMode(ModePlanOnly), prompt)
	result := g.Evaluate(context.Background(), "Bash", false, "ls")
	assert.False(t, promptCalled, "prompt should not be called for Deny")
	assert.Equal(t, Deny, result.Decision)
}

func TestGate_BashScanPassedToPrompt(t *testing.T) {
	var receivedScan *ScanResult
	prompt := func(_ context.Context, _ string, _ string, scan *ScanResult) bool {
		receivedScan = scan
		return true
	}
	g := NewGate(NewCheckerWithMode(ModeDefault), prompt)
	g.Evaluate(context.Background(), "Bash", false, "sudo rm -rf /")
	assert.NotNil(t, receivedScan)
	assert.False(t, receivedScan.Safe)
}
