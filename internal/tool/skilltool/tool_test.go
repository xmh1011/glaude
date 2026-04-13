package skilltool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmh1011/glaude/internal/skill"
)

func newTestRegistry() *skill.Registry {
	reg := skill.NewRegistry()
	reg.Register(&skill.Skill{
		Name:          "greet",
		Description:   "Greet someone",
		WhenToUse:     "When greeting",
		UserInvocable: true,
		Source:         "bundled",
		GetPrompt: func(args string) (string, error) {
			if args != "" {
				return "Say hello to " + args, nil
			}
			return "Say hello", nil
		},
	})
	reg.Register(&skill.Skill{
		Name:          "private",
		Description:   "A non-user skill",
		UserInvocable: false,
		Source:         "bundled",
		GetPrompt:     func(args string) (string, error) { return "internal prompt", nil },
	})
	return reg
}

func TestTool_Name(t *testing.T) {
	tool := &Tool{SkillRegistry: skill.NewRegistry()}
	assert.Equal(t, "Skill", tool.Name())
}

func TestTool_IsReadOnly(t *testing.T) {
	tool := &Tool{SkillRegistry: skill.NewRegistry()}
	assert.True(t, tool.IsReadOnly())
}

func TestTool_Description_IncludesSkills(t *testing.T) {
	tool := &Tool{SkillRegistry: newTestRegistry()}
	desc := tool.Description()
	assert.Contains(t, desc, "greet")
	assert.Contains(t, desc, "private")
}

func TestTool_InputSchema(t *testing.T) {
	tool := &Tool{SkillRegistry: skill.NewRegistry()}
	var schema map[string]interface{}
	err := json.Unmarshal(tool.InputSchema(), &schema)
	require.NoError(t, err)
	assert.Equal(t, "object", schema["type"])
}

func TestTool_Execute_Success(t *testing.T) {
	tool := &Tool{SkillRegistry: newTestRegistry()}
	input, _ := json.Marshal(Input{Skill: "greet", Args: "World"})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "Say hello to World", result)
}

func TestTool_Execute_NoArgs(t *testing.T) {
	tool := &Tool{SkillRegistry: newTestRegistry()}
	input, _ := json.Marshal(Input{Skill: "greet"})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "Say hello", result)
}

func TestTool_Execute_NotFound(t *testing.T) {
	tool := &Tool{SkillRegistry: newTestRegistry()}
	input, _ := json.Marshal(Input{Skill: "nonexistent"})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result, "not found")
	assert.Contains(t, result, "greet")
}

func TestTool_Execute_EmptyName(t *testing.T) {
	tool := &Tool{SkillRegistry: newTestRegistry()}
	input, _ := json.Marshal(Input{})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result, "skill name is required")
}

func TestTool_Execute_CancelledContext(t *testing.T) {
	tool := &Tool{SkillRegistry: newTestRegistry()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	input, _ := json.Marshal(Input{Skill: "greet"})
	_, err := tool.Execute(ctx, input)
	assert.Error(t, err)
}
