package permission

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChecker_ReadOnlyAlwaysAllowed(t *testing.T) {
	for _, mode := range AllModes() {
		c := NewCheckerWithMode(mode)
		result := c.Check("Read", true, "")
		assert.Equal(t, Allow, result.Decision, "mode=%s", mode)
	}
}

func TestChecker_DefaultMode_MutatingNeedsAsk(t *testing.T) {
	c := NewCheckerWithMode(ModeDefault)

	for _, tool := range []string{"Bash", "Edit", "Write"} {
		result := c.Check(tool, false, "")
		assert.Equal(t, Ask, result.Decision, "tool=%s", tool)
	}
}

func TestChecker_AutoEditMode_EditsAllowed(t *testing.T) {
	c := NewCheckerWithMode(ModeAutoEdit)

	assert.Equal(t, Allow, c.Check("Edit", false, "").Decision)
	assert.Equal(t, Allow, c.Check("Write", false, "").Decision)
}

func TestChecker_AutoEditMode_BashNeedsAsk(t *testing.T) {
	c := NewCheckerWithMode(ModeAutoEdit)
	result := c.Check("Bash", false, "ls -la")
	assert.Equal(t, Ask, result.Decision)
}

func TestChecker_PlanOnlyMode_MutatingDenied(t *testing.T) {
	c := NewCheckerWithMode(ModePlanOnly)

	for _, tool := range []string{"Bash", "Edit", "Write"} {
		result := c.Check(tool, false, "")
		assert.Equal(t, Deny, result.Decision, "tool=%s", tool)
	}
}

func TestChecker_PlanOnlyMode_ReadAllowed(t *testing.T) {
	c := NewCheckerWithMode(ModePlanOnly)
	result := c.Check("Read", true, "")
	assert.Equal(t, Allow, result.Decision)
}

func TestChecker_AutoFullMode_EverythingAllowed(t *testing.T) {
	c := NewCheckerWithMode(ModeAutoFull)

	for _, tool := range []string{"Bash", "Edit", "Write", "Read"} {
		result := c.Check(tool, tool == "Read", "")
		assert.Equal(t, Allow, result.Decision, "tool=%s", tool)
	}
}

func TestChecker_SetMode(t *testing.T) {
	c := NewCheckerWithMode(ModeDefault)
	assert.Equal(t, ModeDefault, c.Mode())

	c.SetMode(ModeAutoFull)
	assert.Equal(t, ModeAutoFull, c.Mode())

	result := c.Check("Bash", false, "rm -rf /")
	assert.Equal(t, Allow, result.Decision)
}

func TestCheckResult_HasReason(t *testing.T) {
	c := NewCheckerWithMode(ModePlanOnly)
	result := c.Check("Bash", false, "")
	assert.Contains(t, result.Reason, "plan-only")
	assert.Equal(t, "Bash", result.Tool)
}
