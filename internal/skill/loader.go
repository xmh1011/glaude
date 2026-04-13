package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const skillFileName = "SKILL.md"

// LoadFromDir loads all skills from subdirectories under dir.
// Each subdirectory containing a SKILL.md file becomes a skill
// whose name is the subdirectory name.
func LoadFromDir(dir, source string) ([]*Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skill dir %s: %w", dir, err)
	}

	var skills []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		mdPath := filepath.Join(dir, name, skillFileName)
		data, err := os.ReadFile(mdPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read skill %s: %w", name, err)
		}
		s, err := parseSkillFile(name, source, string(data))
		if err != nil {
			return nil, fmt.Errorf("parse skill %s: %w", name, err)
		}
		skills = append(skills, s)
	}
	return skills, nil
}

// LoadAll loads skills from project and user directories.
// Project skills override user skills when names collide.
func LoadAll(cwd string) ([]*Skill, error) {
	var all []*Skill

	// User-global skills: ~/.glaude/skills/
	home, err := os.UserHomeDir()
	if err == nil {
		userDir := filepath.Join(home, ".glaude", "skills")
		userSkills, err := LoadFromDir(userDir, "user")
		if err != nil {
			return nil, fmt.Errorf("load user skills: %w", err)
		}
		all = append(all, userSkills...)
	}

	// Project-local skills: .glaude/skills/
	if cwd != "" {
		projDir := filepath.Join(cwd, ".glaude", "skills")
		projSkills, err := LoadFromDir(projDir, "project")
		if err != nil {
			return nil, fmt.Errorf("load project skills: %w", err)
		}
		all = append(all, projSkills...)
	}

	return all, nil
}

// parseSkillFile parses a SKILL.md file with optional YAML frontmatter.
func parseSkillFile(name, source, content string) (*Skill, error) {
	fm, body := parseFrontmatter(content)

	desc := fm["description"]
	whenToUse := fm["when_to_use"]
	userInvocable := true
	if v, ok := fm["user-invocable"]; ok {
		userInvocable = parseBool(v, true)
	}

	s := &Skill{
		Name:          name,
		Description:   desc,
		WhenToUse:     whenToUse,
		UserInvocable: userInvocable,
		Source:         source,
		GetPrompt: func(args string) (string, error) {
			return substituteArguments(body, args), nil
		},
	}
	return s, nil
}

// parseFrontmatter extracts YAML-like key: value pairs between --- delimiters.
// Returns the parsed fields and the remaining body.
func parseFrontmatter(content string) (map[string]string, string) {
	fm := make(map[string]string)

	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return fm, content
	}

	// Find the closing ---
	rest := trimmed[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return fm, content
	}

	header := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:]) // skip \n---

	// Parse key: value lines
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		// Remove surrounding quotes if present
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		fm[key] = value
	}

	return fm, body
}

// substituteArguments replaces $ARGUMENTS in the prompt with the given args.
func substituteArguments(body, args string) string {
	return strings.ReplaceAll(body, "$ARGUMENTS", args)
}

// parseBool parses a string as a boolean value.
func parseBool(s string, defaultVal bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	default:
		return defaultVal
	}
}
