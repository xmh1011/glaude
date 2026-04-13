package skill

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeSkill(name, source string, userInvocable bool) *Skill {
	return &Skill{
		Name:          name,
		Description:   name + " description",
		UserInvocable: userInvocable,
		Source:         source,
		GetPrompt:     func(args string) (string, error) { return "prompt:" + args, nil },
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	s := makeSkill("test", "bundled", true)
	reg.Register(s)

	got := reg.Get("test")
	assert.Equal(t, s, got)

	assert.Nil(t, reg.Get("nonexistent"))
}

func TestRegistry_RegisterOverride(t *testing.T) {
	reg := NewRegistry()
	bundled := makeSkill("commit", "bundled", true)
	project := makeSkill("commit", "project", true)

	reg.Register(bundled)
	reg.Register(project)

	got := reg.Get("commit")
	assert.Equal(t, "project", got.Source)
}

func TestRegistry_All_Sorted(t *testing.T) {
	reg := NewRegistry()
	reg.Register(makeSkill("zebra", "bundled", true))
	reg.Register(makeSkill("alpha", "bundled", true))
	reg.Register(makeSkill("middle", "bundled", true))

	all := reg.All()
	assert.Len(t, all, 3)
	assert.Equal(t, "alpha", all[0].Name)
	assert.Equal(t, "middle", all[1].Name)
	assert.Equal(t, "zebra", all[2].Name)
}

func TestRegistry_UserInvocable(t *testing.T) {
	reg := NewRegistry()
	reg.Register(makeSkill("public", "bundled", true))
	reg.Register(makeSkill("private", "bundled", false))
	reg.Register(makeSkill("also-public", "bundled", true))

	ui := reg.UserInvocable()
	assert.Len(t, ui, 2)
	assert.Equal(t, "also-public", ui[0].Name)
	assert.Equal(t, "public", ui[1].Name)
}

func TestRegistry_ForPrompt_Empty(t *testing.T) {
	reg := NewRegistry()
	assert.Equal(t, "", reg.ForPrompt())
}

func TestRegistry_ForPrompt(t *testing.T) {
	reg := NewRegistry()
	s := &Skill{
		Name:          "test",
		Description:   "A test skill",
		WhenToUse:     "When testing",
		UserInvocable: true,
		Source:         "bundled",
	}
	reg.Register(s)

	prompt := reg.ForPrompt()
	assert.Contains(t, prompt, "test")
	assert.Contains(t, prompt, "A test skill")
	assert.Contains(t, prompt, "When testing")
}

// --- Token Budget Tests ---

func TestCharBudget_Default(t *testing.T) {
	assert.Equal(t, DefaultCharBudget, charBudget(0))
}

func TestCharBudget_FromContextWindow(t *testing.T) {
	// 200k tokens × 4 chars/token × 1% = 8000
	assert.Equal(t, 8000, charBudget(200000))
	// 100k tokens × 4 × 1% = 4000
	assert.Equal(t, 4000, charBudget(100000))
}

func TestSkillDescription_Combined(t *testing.T) {
	s := &Skill{Description: "Do stuff", WhenToUse: "When needed"}
	assert.Equal(t, "Do stuff - When needed", skillDescription(s))
}

func TestSkillDescription_NoWhenToUse(t *testing.T) {
	s := &Skill{Description: "Do stuff"}
	assert.Equal(t, "Do stuff", skillDescription(s))
}

func TestSkillDescription_Capped(t *testing.T) {
	s := &Skill{Description: strings.Repeat("x", 300)}
	desc := skillDescription(s)
	assert.Equal(t, MaxListingDescChars, len([]rune(desc)))
	assert.True(t, strings.HasSuffix(desc, "…"))
}

func TestTruncateStr(t *testing.T) {
	assert.Equal(t, "hello", truncateStr("hello", 10))
	assert.Equal(t, "hel…", truncateStr("hello", 4))
	assert.Equal(t, "…", truncateStr("hello", 1))
	assert.Equal(t, "", truncateStr("hello", 0))
	assert.Equal(t, "hello", truncateStr("hello", 5))
}

func TestFormatSkillsWithinBudget_Level1_FullDescriptions(t *testing.T) {
	skills := []*Skill{
		{Name: "a", Description: "short", Source: "bundled"},
		{Name: "b", Description: "also short", Source: "user"},
	}
	// Large budget: everything fits
	result := formatSkillsWithinBudget(skills, 10000)
	assert.Contains(t, result, "- a: short")
	assert.Contains(t, result, "- b: also short")
}

func TestFormatSkillsWithinBudget_Level2_TruncatedDescriptions(t *testing.T) {
	bundled := &Skill{Name: "bundled-skill", Description: "Bundled description stays full", Source: "bundled"}
	user := &Skill{
		Name:        "user-skill",
		Description: "This is a very long description that should get truncated to fit within the budget constraints",
		Source:       "user",
	}
	skills := []*Skill{bundled, user}

	// Budget tight enough to force truncation but not names-only
	// bundled full entry: "- bundled-skill: Bundled description stays full" = 48 chars
	// Set budget so user skill description must be truncated
	result := formatSkillsWithinBudget(skills, 100)
	// Bundled should keep full description
	assert.Contains(t, result, "Bundled description stays full")
	// User skill description should be truncated (has ellipsis)
	lines := strings.Split(result, "\n")
	var userLine string
	for _, l := range lines {
		if strings.HasPrefix(l, "- user-skill") {
			userLine = l
		}
	}
	assert.NotEmpty(t, userLine, "user-skill line should exist")
	// The description should be shorter than the original
	assert.Less(t, len(userLine), len("- user-skill: "+user.Description))
}

func TestFormatSkillsWithinBudget_Level3_NamesOnly(t *testing.T) {
	bundled := &Skill{Name: "b", Description: "Bundled keeps desc", Source: "bundled"}
	user1 := &Skill{Name: "u1", Description: "User desc one", Source: "user"}
	user2 := &Skill{Name: "u2", Description: "User desc two", Source: "project"}
	skills := []*Skill{bundled, user1, user2}

	// Very tight budget: only names for non-bundled
	// bundled full = "- b: Bundled keeps desc" = 23 chars + 1 newline = 24
	// That leaves almost nothing for user skills -> names only
	result := formatSkillsWithinBudget(skills, 40)
	// Bundled keeps full description
	assert.Contains(t, result, "- b: Bundled keeps desc")
	// Non-bundled are names only
	assert.Contains(t, result, "- u1\n")
	assert.Contains(t, result, "- u2")
	assert.NotContains(t, result, "User desc one")
	assert.NotContains(t, result, "User desc two")
}

func TestFormatSkillsWithinBudget_AllBundled_NeverTruncated(t *testing.T) {
	skills := []*Skill{
		{Name: "a", Description: "desc-a", Source: "bundled"},
		{Name: "b", Description: "desc-b", Source: "bundled"},
	}
	// Even with tight budget, all-bundled returns full
	result := formatSkillsWithinBudget(skills, 10)
	assert.Contains(t, result, "- a: desc-a")
	assert.Contains(t, result, "- b: desc-b")
}

func TestForPromptWithBudget(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Skill{Name: "test", Description: "Test skill", Source: "bundled"})

	result := reg.ForPromptWithBudget(200000)
	assert.Contains(t, result, "# Available Skills")
	assert.Contains(t, result, "test")
	assert.Contains(t, result, "Test skill")
}
