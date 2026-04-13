// Package skilltool implements the Skill tool that allows the LLM to invoke
// registered skills by name, expanding their prompt templates.
package skilltool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xmh1011/glaude/internal/skill"
)

// Tool implements the tool.Tool interface for skill invocation.
type Tool struct {
	SkillRegistry *skill.Registry
}

// Input defines the expected JSON input from the LLM.
type Input struct {
	Skill string `json:"skill"`
	Args  string `json:"args"`
}

// Name returns the tool identifier.
func (t *Tool) Name() string { return "Skill" }

// Description returns a dynamic description including the list of available skills.
func (t *Tool) Description() string {
	desc := `Execute a skill within the main conversation.

When users ask you to perform tasks, check if any of the available skills match. Skills provide specialized capabilities and domain knowledge.

When users reference a "slash command" or "/<something>" (e.g., "/commit", "/simplify"), they are referring to a skill. Use this tool to invoke it.

How to invoke:
- Use this tool with the skill name and optional arguments
- Examples:
  - skill: "commit" - invoke the commit skill
  - skill: "simplify", args: "focus on error handling" - invoke with arguments

Important:
- Available skills are listed below
- When a skill matches the user's request, invoke the relevant Skill tool BEFORE generating any other response
- Do not invoke a skill that is already running
`
	if t.SkillRegistry != nil {
		listing := t.SkillRegistry.ForPrompt()
		if listing != "" {
			desc += "\n" + listing
		}
	}
	return desc
}

// InputSchema returns the JSON Schema for the tool input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"skill": {"type": "string", "description": "The skill name. E.g., \"commit\", \"simplify\""},
			"args": {"type": "string", "description": "Optional arguments for the skill"}
		},
		"required": ["skill"]
	}`)
}

// IsReadOnly returns true because the Skill tool itself only expands prompts.
func (t *Tool) IsReadOnly() bool { return true }

// Execute looks up the skill by name, expands its prompt with the given args,
// and returns the expanded prompt text as the tool result.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse Skill input: %w", err)
	}

	if in.Skill == "" {
		return "Error: skill name is required", nil
	}

	s := t.SkillRegistry.Get(in.Skill)
	if s == nil {
		available := ""
		for _, sk := range t.SkillRegistry.All() {
			available += fmt.Sprintf("\n  - %s: %s", sk.Name, sk.Description)
		}
		return fmt.Sprintf("Error: skill %q not found. Available skills:%s", in.Skill, available), nil
	}

	prompt, err := s.GetPrompt(in.Args)
	if err != nil {
		return fmt.Sprintf("Error expanding skill %q: %s", in.Skill, err), nil
	}

	return prompt, nil
}
