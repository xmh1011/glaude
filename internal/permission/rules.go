package permission

import (
	"regexp"
	"strings"
)

// RuleType identifies how a rule pattern is matched against commands.
type RuleType int

const (
	RuleExact    RuleType = iota // exact string match
	RulePrefix                   // "command:*" matches command and command + args
	RuleWildcard                 // "git * --force" with * as wildcard
)

// Rule represents a permission rule for a tool invocation.
// Rules can be of type Bash(command) or Tool(name).
type Rule struct {
	Pattern  string   // the original rule pattern
	Type     RuleType // how the pattern is matched
	Behavior Decision // Allow, Deny, or Ask
	Tool     string   // tool name this rule applies to (e.g. "Bash", "Edit")
}

// RuleSet holds categorized permission rules, evaluated in priority order:
// deny > ask > allow, matching Claude Code's pipeline.
type RuleSet struct {
	DenyRules  []Rule
	AskRules   []Rule
	AllowRules []Rule
}

// NewRuleSet creates an empty RuleSet.
func NewRuleSet() *RuleSet {
	return &RuleSet{}
}

// AddRule adds a rule to the appropriate category based on its behavior.
func (rs *RuleSet) AddRule(r Rule) {
	switch r.Behavior {
	case Deny:
		rs.DenyRules = append(rs.DenyRules, r)
	case Ask:
		rs.AskRules = append(rs.AskRules, r)
	case Allow:
		rs.AllowRules = append(rs.AllowRules, r)
	}
}

// ParseRule parses a rule string into a Rule struct.
// Supported formats:
//   - "git status"       → exact match
//   - "npm:*"            → prefix match (npm and npm <args>)
//   - "git * --force"    → wildcard match
func ParseRule(pattern string, behavior Decision, toolName string) Rule {
	// Check for prefix pattern: "command:*"
	if strings.HasSuffix(pattern, ":*") {
		return Rule{
			Pattern:  strings.TrimSuffix(pattern, ":*"),
			Type:     RulePrefix,
			Behavior: behavior,
			Tool:     toolName,
		}
	}

	// Check for wildcard pattern: contains unescaped *
	if containsUnescapedWildcard(pattern) {
		return Rule{
			Pattern:  pattern,
			Type:     RuleWildcard,
			Behavior: behavior,
			Tool:     toolName,
		}
	}

	// Default: exact match
	return Rule{
		Pattern:  pattern,
		Type:     RuleExact,
		Behavior: behavior,
		Tool:     toolName,
	}
}

// EvaluateBashRule checks a bash command against the rule set.
// Returns the decision and whether a rule matched.
// Priority: deny > ask > allow.
func (rs *RuleSet) EvaluateBashRule(command string) (Decision, string, bool) {
	// Normalize command for matching
	normalizedCmd := strings.TrimSpace(command)

	// Step 1: Check deny rules (aggressive normalization)
	aggressiveCmd := stripAllEnvVars(normalizedCmd)
	for _, rule := range rs.DenyRules {
		if rule.Tool != "Bash" {
			continue
		}
		if matchRule(rule, aggressiveCmd) {
			return Deny, "denied by rule: " + rule.Pattern, true
		}
	}

	// Step 2: Check ask rules (aggressive normalization)
	for _, rule := range rs.AskRules {
		if rule.Tool != "Bash" {
			continue
		}
		if matchRule(rule, aggressiveCmd) {
			return Ask, "ask rule: " + rule.Pattern, true
		}
	}

	// Step 3: Check allow rules (conservative normalization)
	conservativeCmd := stripSafeWrappers(normalizedCmd)
	for _, rule := range rs.AllowRules {
		if rule.Tool != "Bash" {
			continue
		}
		// Allow rules do NOT match compound commands
		if isCompoundCommand(conservativeCmd) {
			continue
		}
		if matchRule(rule, conservativeCmd) {
			return Allow, "allowed by rule: " + rule.Pattern, true
		}
	}

	// No rule matched
	return Ask, "", false
}

// EvaluateToolRule checks a non-Bash tool call against the rule set.
func (rs *RuleSet) EvaluateToolRule(toolName string) (Decision, string, bool) {
	for _, rule := range rs.DenyRules {
		if rule.Tool == toolName && rule.Type == RuleExact {
			return Deny, "denied by rule: " + rule.Pattern, true
		}
	}
	for _, rule := range rs.AskRules {
		if rule.Tool == toolName && rule.Type == RuleExact {
			return Ask, "ask rule: " + rule.Pattern, true
		}
	}
	for _, rule := range rs.AllowRules {
		if rule.Tool == toolName && rule.Type == RuleExact {
			return Allow, "allowed by rule: " + rule.Pattern, true
		}
	}
	return Ask, "", false
}

// matchRule checks if a command matches a specific rule.
func matchRule(rule Rule, command string) bool {
	switch rule.Type {
	case RuleExact:
		return command == rule.Pattern
	case RulePrefix:
		return command == rule.Pattern ||
			strings.HasPrefix(command, rule.Pattern+" ")
	case RuleWildcard:
		return matchWildcard(rule.Pattern, command)
	}
	return false
}

// matchWildcard converts a wildcard pattern to a regex and tests the command.
// "*" matches any sequence of characters (non-greedy for embedded wildcards).
func matchWildcard(pattern, command string) bool {
	// Escape regex metacharacters except *
	var re strings.Builder
	re.WriteString("^")

	parts := strings.Split(pattern, "*")
	for i, part := range parts {
		re.WriteString(regexp.QuoteMeta(part))
		if i < len(parts)-1 {
			re.WriteString(".*")
		}
	}
	re.WriteString("$")

	matched, err := regexp.MatchString(re.String(), command)
	return err == nil && matched
}

// containsUnescapedWildcard checks for * not preceded by backslash and not in :* suffix.
func containsUnescapedWildcard(pattern string) bool {
	if strings.HasSuffix(pattern, ":*") {
		return false
	}
	for i, ch := range pattern {
		if ch == '*' && (i == 0 || pattern[i-1] != '\\') {
			return true
		}
	}
	return false
}

// isCompoundCommand returns true if the command contains shell operators.
func isCompoundCommand(cmd string) bool {
	// Simple heuristic: check for &&, ||, |, ; outside of quotes
	inSingle := false
	inDouble := false
	for i := 0; i < len(cmd); i++ {
		ch := cmd[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case !inSingle && !inDouble:
			if ch == ';' || ch == '|' || ch == '\n' {
				return true
			}
			if ch == '&' && i+1 < len(cmd) && cmd[i+1] == '&' {
				return true
			}
		}
	}
	return false
}

// splitCompoundCommand splits a command on shell operators (&&, ||, |, ;)
// respecting quoted strings. Returns individual subcommands.
func splitCompoundCommand(cmd string) []string {
	var parts []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	flush := func() {
		s := strings.TrimSpace(current.String())
		if s != "" {
			parts = append(parts, s)
		}
		current.Reset()
	}

	for i := 0; i < len(cmd); i++ {
		ch := cmd[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(ch)
		case ch == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(ch)
		case !inSingle && !inDouble:
			switch {
			case ch == '&' && i+1 < len(cmd) && cmd[i+1] == '&':
				flush()
				i++ // skip second &
			case ch == '|' && i+1 < len(cmd) && cmd[i+1] == '|':
				flush()
				i++ // skip second |
			case ch == '|':
				flush()
			case ch == ';':
				flush()
			case ch == '\n':
				flush()
			default:
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}
	flush()
	return parts
}

// safeEnvVars are environment variable prefixes that are safe to strip
// when evaluating allow rules. Matching Claude Code's approach.
var safeEnvVars = map[string]bool{
	"NODE_ENV": true, "GOOS": true, "GOARCH": true,
	"CGO_ENABLED": true, "RUST_LOG": true, "RUST_BACKTRACE": true,
	"DEBUG": true, "VERBOSE": true, "CI": true,
	"PATH": true, "HOME": true, "USER": true,
	"LANG": true, "LC_ALL": true, "TERM": true,
	"EDITOR": true, "VISUAL": true, "PAGER": true,
	"GOPATH": true, "GOROOT": true, "GOBIN": true,
	"PYTHONPATH": true, "VIRTUAL_ENV": true,
	"npm_config_registry": true, "YARN_CACHE_FOLDER": true,
}

// safeEnvVarPattern matches "VAR=value " at the start.
var safeEnvVarPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=([A-Za-z0-9_./:@-]*)\s+`)

// safeWrapperCommands are commands that are safe to strip from the beginning.
var safeWrapperCommands = []string{"timeout", "time", "nice", "nohup", "stdbuf"}

// stripSafeWrappers performs conservative command normalization for allow rules.
// Only strips known-safe env vars and wrapper commands.
func stripSafeWrappers(cmd string) string {
	result := cmd
	changed := true
	for changed {
		changed = false
		// Strip safe env vars
		for {
			m := safeEnvVarPattern.FindStringSubmatch(result)
			if m == nil {
				break
			}
			varName := m[1]
			if !safeEnvVars[varName] {
				break
			}
			result = strings.TrimPrefix(result, m[0])
			changed = true
		}
		// Strip safe wrapper commands
		for _, wrapper := range safeWrapperCommands {
			prefix := wrapper + " "
			if strings.HasPrefix(result, prefix) {
				result = strings.TrimPrefix(result, prefix)
				result = strings.TrimSpace(result)
				// timeout has a numeric argument
				if wrapper == "timeout" || wrapper == "stdbuf" {
					// Skip the next "word" (the argument)
					if idx := strings.Index(result, " "); idx > 0 {
						result = strings.TrimSpace(result[idx:])
					}
				}
				changed = true
			}
		}
	}
	return result
}

// anyEnvVarPattern matches any "VAR=value " at the start (aggressive).
var anyEnvVarPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=(?:'[^']*'|"[^"]*"|[^\s]*)\s+`)

// stripAllEnvVars performs aggressive command normalization for deny/ask rules.
// Strips ALL leading env var assignments and safe wrappers.
func stripAllEnvVars(cmd string) string {
	result := cmd
	changed := true
	for changed {
		changed = false
		// Strip any env var
		for {
			m := anyEnvVarPattern.FindString(result)
			if m == "" {
				break
			}
			result = strings.TrimPrefix(result, m)
			changed = true
		}
		// Also strip safe wrappers
		for _, wrapper := range safeWrapperCommands {
			prefix := wrapper + " "
			if strings.HasPrefix(result, prefix) {
				result = strings.TrimPrefix(result, prefix)
				result = strings.TrimSpace(result)
				if wrapper == "timeout" || wrapper == "stdbuf" {
					if idx := strings.Index(result, " "); idx > 0 {
						result = strings.TrimSpace(result[idx:])
					}
				}
				changed = true
			}
		}
	}
	return result
}
