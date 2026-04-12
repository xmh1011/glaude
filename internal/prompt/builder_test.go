package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuilder_Build(t *testing.T) {
	b := NewBuilder()
	result := b.Build()

	assert.Contains(t, result, "AI coding agent")
	assert.Contains(t, result, "# Rules")
	assert.Contains(t, result, "# Environment")
	assert.Contains(t, result, "Platform:")
}

func TestBuilder_WithCustomInstructions(t *testing.T) {
	b := NewBuilder().WithCustomInstructions("Always use Go.")
	result := b.Build()

	assert.Contains(t, result, "Always use Go.")
	assert.Contains(t, result, "# Custom Instructions")
}

func TestBuilder_SegmentOrder(t *testing.T) {
	b := NewBuilder().WithCustomInstructions("custom here")
	result := b.Build()

	// Identity should come before Rules
	idxIdentity := strings.Index(result, "AI coding agent")
	idxRules := strings.Index(result, "# Rules")
	idxCustom := strings.Index(result, "# Custom Instructions")
	idxEnv := strings.Index(result, "# Environment")

	assert.Greater(t, idxRules, idxIdentity, "rules should come after identity")
	assert.Greater(t, idxCustom, idxRules, "custom should come after rules")
	assert.Greater(t, idxEnv, idxCustom, "environment should come after custom")
}
