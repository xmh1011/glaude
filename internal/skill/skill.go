// Package skill defines the Skill system — reusable prompt templates that can be
// invoked by users via slash commands or by the LLM via the Skill Tool.
package skill

import (
	"fmt"
	"sort"
	"strings"
)

// Budget constants for skill listing token control.
const (
	// SkillBudgetContextPercent is the fraction of context window allocated to skill listings.
	SkillBudgetContextPercent = 0.01

	// CharsPerToken is the approximate character-to-token ratio.
	CharsPerToken = 4

	// DefaultCharBudget is the fallback budget when context window is unknown (1% of 200k × 4).
	DefaultCharBudget = 8000

	// MaxListingDescChars is the per-entry hard cap for description + whenToUse.
	MaxListingDescChars = 250

	// minDescLength is the threshold below which descriptions are dropped entirely
	// for non-bundled skills (names-only mode).
	minDescLength = 20
)

// Skill defines a callable prompt template.
type Skill struct {
	// Name is the unique identifier, used as /name for slash commands.
	Name string

	// Description is a short human-readable summary shown in /help and tool listings.
	Description string

	// WhenToUse provides guidance to the LLM on when to invoke this skill.
	// Injected into the Skill Tool prompt if non-empty.
	WhenToUse string

	// UserInvocable indicates whether users can trigger this skill via /name.
	UserInvocable bool

	// Source indicates where this skill was loaded from: "bundled", "user", or "project".
	Source string

	// GetPrompt returns the expanded prompt text for a given argument string.
	GetPrompt func(args string) (string, error)
}

// Registry manages registered skills with name-based lookup.
// Later registrations with the same name override earlier ones,
// enabling the priority chain: bundled < user < project.
type Registry struct {
	skills map[string]*Skill
}

// NewRegistry creates an empty skill registry.
func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]*Skill)}
}

// Register adds a skill to the registry.
// If a skill with the same name already exists, it is replaced.
func (r *Registry) Register(s *Skill) {
	r.skills[s.Name] = s
}

// Get returns the skill with the given name, or nil if not found.
func (r *Registry) Get(name string) *Skill {
	return r.skills[name]
}

// All returns all registered skills sorted alphabetically by name.
func (r *Registry) All() []*Skill {
	result := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// UserInvocable returns only skills that users can invoke via slash commands,
// sorted alphabetically by name.
func (r *Registry) UserInvocable() []*Skill {
	var result []*Skill
	for _, s := range r.skills {
		if s.UserInvocable {
			result = append(result, s)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// ForPrompt generates a text listing of skills suitable for injection into
// the LLM system prompt, using the default character budget.
func (r *Registry) ForPrompt() string {
	return r.ForPromptWithBudget(0)
}

// ForPromptWithBudget generates a text listing of skills with three-level
// degradation to fit within a character budget.
//
// Budget calculation: contextWindowTokens × 4 × 1%, default 8000 chars.
//
// Three degradation levels:
//  1. Full: complete description + whenToUse (capped at 250 chars per entry)
//  2. Truncated: bundled skills keep full descriptions, others are truncated
//  3. Names-only: bundled skills keep full descriptions, others show only name
func (r *Registry) ForPromptWithBudget(contextWindowTokens int) string {
	skills := r.All()
	if len(skills) == 0 {
		return ""
	}

	budget := charBudget(contextWindowTokens)
	listing := formatSkillsWithinBudget(skills, budget)

	var b strings.Builder
	b.WriteString("# Available Skills\n\n")
	b.WriteString("Skills can be invoked using the Skill tool with `skill: \"name\"` and optional `args`.\n")
	b.WriteString("User-invocable skills can also be triggered via `/name` slash commands.\n\n")
	b.WriteString(listing)
	b.WriteString("\n")

	return b.String()
}

// charBudget calculates the character budget for skill listings.
func charBudget(contextWindowTokens int) int {
	if contextWindowTokens > 0 {
		return int(float64(contextWindowTokens) * CharsPerToken * SkillBudgetContextPercent)
	}
	return DefaultCharBudget
}

// skillDescription returns the combined description + whenToUse for a skill,
// capped at MaxListingDescChars runes.
func skillDescription(s *Skill) string {
	desc := s.Description
	if s.WhenToUse != "" {
		desc = desc + " - " + s.WhenToUse
	}
	runes := []rune(desc)
	if len(runes) > MaxListingDescChars {
		return string(runes[:MaxListingDescChars-1]) + "…"
	}
	return desc
}

// formatSkillFull formats a single skill entry with its full description.
func formatSkillFull(s *Skill) string {
	desc := skillDescription(s)
	if desc != "" {
		return fmt.Sprintf("- %s: %s", s.Name, desc)
	}
	return fmt.Sprintf("- %s", s.Name)
}

// truncateStr truncates a string to maxLen characters with an ellipsis.
func truncateStr(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}

// formatSkillsWithinBudget implements three-level degradation:
//  1. Full descriptions if total fits within budget
//  2. Truncated descriptions for non-bundled, full for bundled
//  3. Names-only for non-bundled, full for bundled
func formatSkillsWithinBudget(skills []*Skill, budget int) string {
	// Level 1: try full descriptions
	fullEntries := make([]string, len(skills))
	fullTotal := 0
	for i, s := range skills {
		fullEntries[i] = formatSkillFull(s)
		fullTotal += len(fullEntries[i])
	}
	// Account for newlines between entries
	if len(skills) > 1 {
		fullTotal += len(skills) - 1
	}

	if fullTotal <= budget {
		return strings.Join(fullEntries, "\n")
	}

	// Partition into bundled (never truncated) and rest
	bundledSet := make(map[int]bool)
	var restIndices []int
	bundledChars := 0

	for i, s := range skills {
		if s.Source == "bundled" {
			bundledSet[i] = true
			bundledChars += len(fullEntries[i]) + 1 // +1 for newline
		} else {
			restIndices = append(restIndices, i)
		}
	}

	if len(restIndices) == 0 {
		// All bundled — nothing to truncate
		return strings.Join(fullEntries, "\n")
	}

	remainingBudget := budget - bundledChars

	// Calculate per-entry overhead for non-bundled: "- name: " prefix
	restNameOverhead := 0
	for _, i := range restIndices {
		restNameOverhead += len(skills[i].Name) + 4 // "- " + name + ": "
	}
	restNameOverhead += len(restIndices) - 1 // newlines between rest entries

	availableForDescs := remainingBudget - restNameOverhead
	maxDescLen := 0
	if len(restIndices) > 0 {
		maxDescLen = availableForDescs / len(restIndices)
	}

	// Level 3: names-only for non-bundled if per-desc budget is too small
	if maxDescLen < minDescLength {
		entries := make([]string, len(skills))
		for i, s := range skills {
			if bundledSet[i] {
				entries[i] = fullEntries[i]
			} else {
				entries[i] = fmt.Sprintf("- %s", s.Name)
			}
		}
		return strings.Join(entries, "\n")
	}

	// Level 2: truncated descriptions for non-bundled
	entries := make([]string, len(skills))
	for i, s := range skills {
		if bundledSet[i] {
			entries[i] = fullEntries[i]
		} else {
			desc := truncateStr(skillDescription(s), maxDescLen)
			if desc != "" {
				entries[i] = fmt.Sprintf("- %s: %s", s.Name, desc)
			} else {
				entries[i] = fmt.Sprintf("- %s", s.Name)
			}
		}
	}
	return strings.Join(entries, "\n")
}
