package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFrontmatter_Full(t *testing.T) {
	content := `---
description: My skill
when_to_use: When needed
user-invocable: true
---
Do the thing with $ARGUMENTS`

	fm, body := parseFrontmatter(content)
	assert.Equal(t, "My skill", fm["description"])
	assert.Equal(t, "When needed", fm["when_to_use"])
	assert.Equal(t, "true", fm["user-invocable"])
	assert.Equal(t, "Do the thing with $ARGUMENTS", body)
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "Just a plain prompt"
	fm, body := parseFrontmatter(content)
	assert.Empty(t, fm)
	assert.Equal(t, content, body)
}

func TestParseFrontmatter_Partial(t *testing.T) {
	content := `---
description: Only desc
---
Body here`

	fm, body := parseFrontmatter(content)
	assert.Equal(t, "Only desc", fm["description"])
	assert.Empty(t, fm["when_to_use"])
	assert.Equal(t, "Body here", body)
}

func TestParseFrontmatter_QuotedValues(t *testing.T) {
	content := `---
description: "A quoted value"
when_to_use: 'single quoted'
---
Body`

	fm, _ := parseFrontmatter(content)
	assert.Equal(t, "A quoted value", fm["description"])
	assert.Equal(t, "single quoted", fm["when_to_use"])
}

func TestSubstituteArguments(t *testing.T) {
	body := "Review the code in $ARGUMENTS and suggest improvements"
	result := substituteArguments(body, "main.go")
	assert.Equal(t, "Review the code in main.go and suggest improvements", result)
}

func TestSubstituteArguments_NoPlaceholder(t *testing.T) {
	body := "Do something"
	result := substituteArguments(body, "args")
	assert.Equal(t, "Do something", result)
}

func TestSubstituteArguments_Empty(t *testing.T) {
	body := "Review $ARGUMENTS"
	result := substituteArguments(body, "")
	assert.Equal(t, "Review ", result)
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"yes", true},
		{"1", true},
		{"false", false},
		{"False", false},
		{"no", false},
		{"0", false},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.expected, parseBool(tc.input, false), "input: %s", tc.input)
	}
	assert.True(t, parseBool("invalid", true))
	assert.False(t, parseBool("invalid", false))
}

func TestLoadFromDir(t *testing.T) {
	dir := t.TempDir()

	// Create a skill directory
	skillDir := filepath.Join(dir, "greet")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
description: Greet the user
when_to_use: When saying hello
user-invocable: false
---
Say hello to $ARGUMENTS!`), 0o644))

	// Create another skill without frontmatter
	skillDir2 := filepath.Join(dir, "simple")
	require.NoError(t, os.MkdirAll(skillDir2, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir2, "SKILL.md"), []byte("Just do it"), 0o644))

	// Create a non-skill directory (no SKILL.md)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "empty"), 0o755))

	skills, err := LoadFromDir(dir, "project")
	require.NoError(t, err)
	assert.Len(t, skills, 2)

	// Find the greet skill
	var greet, simple *Skill
	for _, s := range skills {
		switch s.Name {
		case "greet":
			greet = s
		case "simple":
			simple = s
		}
	}

	require.NotNil(t, greet)
	assert.Equal(t, "Greet the user", greet.Description)
	assert.Equal(t, "When saying hello", greet.WhenToUse)
	assert.False(t, greet.UserInvocable)
	assert.Equal(t, "project", greet.Source)
	prompt, err := greet.GetPrompt("World")
	require.NoError(t, err)
	assert.Equal(t, "Say hello to World!", prompt)

	require.NotNil(t, simple)
	assert.Empty(t, simple.Description)
	assert.True(t, simple.UserInvocable) // default
	prompt, err = simple.GetPrompt("")
	require.NoError(t, err)
	assert.Equal(t, "Just do it", prompt)
}

func TestLoadFromDir_NotExist(t *testing.T) {
	skills, err := LoadFromDir("/nonexistent/path", "user")
	require.NoError(t, err)
	assert.Nil(t, skills)
}

func TestLoadAll(t *testing.T) {
	dir := t.TempDir()

	// Create project-local skill
	projSkillDir := filepath.Join(dir, ".glaude", "skills", "proj-skill")
	require.NoError(t, os.MkdirAll(projSkillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projSkillDir, "SKILL.md"), []byte(`---
description: Project skill
---
Project prompt`), 0o644))

	skills, err := LoadAll(dir)
	require.NoError(t, err)

	// Should find at least the project skill
	var found bool
	for _, s := range skills {
		if s.Name == "proj-skill" {
			found = true
			assert.Equal(t, "project", s.Source)
			assert.Equal(t, "Project skill", s.Description)
		}
	}
	assert.True(t, found, "project skill should be loaded")
}

func TestParseSkillFile(t *testing.T) {
	content := `---
description: Test skill
when_to_use: Testing
user-invocable: false
---
Execute $ARGUMENTS`

	s, err := parseSkillFile("test", "user", content)
	require.NoError(t, err)
	assert.Equal(t, "test", s.Name)
	assert.Equal(t, "Test skill", s.Description)
	assert.Equal(t, "Testing", s.WhenToUse)
	assert.False(t, s.UserInvocable)
	assert.Equal(t, "user", s.Source)

	prompt, err := s.GetPrompt("foo")
	require.NoError(t, err)
	assert.Equal(t, "Execute foo", prompt)
}
