package permission

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRule_Exact(t *testing.T) {
	r := ParseRule("git status", Allow, "Bash")
	assert.Equal(t, RuleExact, r.Type)
	assert.Equal(t, "git status", r.Pattern)
}

func TestParseRule_Prefix(t *testing.T) {
	r := ParseRule("npm:*", Allow, "Bash")
	assert.Equal(t, RulePrefix, r.Type)
	assert.Equal(t, "npm", r.Pattern)
}

func TestParseRule_Wildcard(t *testing.T) {
	r := ParseRule("git * --force", Deny, "Bash")
	assert.Equal(t, RuleWildcard, r.Type)
	assert.Equal(t, "git * --force", r.Pattern)
}

func TestRuleSet_DenyOverridesAllow(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(ParseRule("rm -rf /", Deny, "Bash"))
	rs.AddRule(ParseRule("rm:*", Allow, "Bash"))

	decision, _, matched := rs.EvaluateBashRule("rm -rf /")
	assert.True(t, matched)
	assert.Equal(t, Deny, decision)
}

func TestRuleSet_ExactMatch(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(ParseRule("git status", Allow, "Bash"))

	decision, _, matched := rs.EvaluateBashRule("git status")
	assert.True(t, matched)
	assert.Equal(t, Allow, decision)

	// Should not match partial
	_, _, matched = rs.EvaluateBashRule("git status -s")
	assert.False(t, matched)
}

func TestRuleSet_PrefixMatch(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(ParseRule("npm:*", Allow, "Bash"))

	// Bare command
	decision, _, matched := rs.EvaluateBashRule("npm")
	assert.True(t, matched)
	assert.Equal(t, Allow, decision)

	// Command with args
	decision, _, matched = rs.EvaluateBashRule("npm run build")
	assert.True(t, matched)
	assert.Equal(t, Allow, decision)

	// Should not match different command
	_, _, matched = rs.EvaluateBashRule("npx something")
	assert.False(t, matched)
}

func TestRuleSet_WildcardMatch(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(ParseRule("git push * --force", Deny, "Bash"))

	decision, _, matched := rs.EvaluateBashRule("git push origin --force")
	assert.True(t, matched)
	assert.Equal(t, Deny, decision)

	// Should not match without --force
	_, _, matched = rs.EvaluateBashRule("git push origin main")
	assert.False(t, matched)
}

func TestRuleSet_AllowBlocksCompoundCommands(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(ParseRule("ls:*", Allow, "Bash"))

	// Simple command: allowed
	decision, _, matched := rs.EvaluateBashRule("ls -la")
	assert.True(t, matched)
	assert.Equal(t, Allow, decision)

	// Compound command: allow rule should NOT match
	_, _, matched = rs.EvaluateBashRule("ls -la && rm -rf /")
	assert.False(t, matched)
}

func TestRuleSet_DenyMatchesCompoundCommands(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(ParseRule("rm -rf /", Deny, "Bash"))

	// Deny rules use aggressive normalization - no compound check
	decision, _, matched := rs.EvaluateBashRule("rm -rf /")
	assert.True(t, matched)
	assert.Equal(t, Deny, decision)
}

func TestRuleSet_ToolRule(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(Rule{Pattern: "Edit", Type: RuleExact, Behavior: Allow, Tool: "Edit"})

	decision, _, matched := rs.EvaluateToolRule("Edit")
	assert.True(t, matched)
	assert.Equal(t, Allow, decision)

	_, _, matched = rs.EvaluateToolRule("Write")
	assert.False(t, matched)
}

func TestStripSafeWrappers(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"git status", "git status"},
		{"NODE_ENV=production npm build", "npm build"},
		{"timeout 30 npm test", "npm test"},
		{"time git status", "git status"},
		{"nice npm run build", "npm run build"},
		{"GOOS=linux GOARCH=amd64 go build", "go build"},
		// Unsafe env var should NOT be stripped
		{"DOCKER_HOST=tcp://evil docker ps", "DOCKER_HOST=tcp://evil docker ps"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, stripSafeWrappers(tt.input))
		})
	}
}

func TestStripAllEnvVars(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"git status", "git status"},
		{"FOO=bar git status", "git status"},
		{"DOCKER_HOST=tcp://evil docker ps", "docker ps"},
		{"A=1 B=2 C=3 cmd", "cmd"},
		{"FOO='bar baz' cmd", "cmd"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, stripAllEnvVars(tt.input))
		})
	}
}

func TestIsCompoundCommand(t *testing.T) {
	assert.False(t, isCompoundCommand("git status"))
	assert.True(t, isCompoundCommand("ls && rm -rf /"))
	assert.True(t, isCompoundCommand("ls || echo fail"))
	assert.True(t, isCompoundCommand("ls | grep foo"))
	assert.True(t, isCompoundCommand("ls; rm /"))
	// Quoted operators should NOT trigger
	assert.False(t, isCompoundCommand(`echo "hello && world"`))
}

func TestSplitCompoundCommand(t *testing.T) {
	parts := splitCompoundCommand("ls -la && cd /tmp || echo fail")
	assert.Equal(t, []string{"ls -la", "cd /tmp", "echo fail"}, parts)

	// Single command
	parts = splitCompoundCommand("git status")
	assert.Equal(t, []string{"git status"}, parts)

	// Pipe
	parts = splitCompoundCommand("ls | grep foo")
	assert.Equal(t, []string{"ls", "grep foo"}, parts)
}

func TestChecker_WithRules(t *testing.T) {
	c := NewCheckerWithMode(ModeDefault)
	c.Rules().AddRule(ParseRule("git status", Allow, "Bash"))
	c.Rules().AddRule(ParseRule("rm -rf /", Deny, "Bash"))

	// Allowed by rule
	result := c.Check("Bash", false, "git status")
	assert.Equal(t, Allow, result.Decision)

	// Denied by rule
	result = c.Check("Bash", false, "rm -rf /")
	assert.Equal(t, Deny, result.Decision)

	// No rule match, falls through to mode-based check
	result = c.Check("Bash", false, "cat /etc/passwd")
	assert.Equal(t, Ask, result.Decision) // ModeDefault -> Ask
}

func TestChecker_RulesDenyOverridesMode(t *testing.T) {
	// Even in AutoFull mode, explicit deny rules should be checked
	c := NewCheckerWithMode(ModeAutoFull)
	c.Rules().AddRule(ParseRule("rm -rf /", Deny, "Bash"))

	result := c.Check("Bash", false, "rm -rf /")
	assert.Equal(t, Deny, result.Decision, "deny rule should override auto-full mode")
}

func TestAsymmetricNormalization(t *testing.T) {
	rs := NewRuleSet()
	// Deny rule: should match even with env var prefix
	rs.AddRule(ParseRule("dangerous_cmd", Deny, "Bash"))
	// Allow rule: should NOT match with unknown env var prefix
	rs.AddRule(ParseRule("safe_cmd", Allow, "Bash"))

	// Deny matches with env var
	decision, _, matched := rs.EvaluateBashRule("FOO=bar dangerous_cmd")
	assert.True(t, matched)
	assert.Equal(t, Deny, decision)

	// Allow does NOT match with unknown env var (conservative)
	_, _, matched = rs.EvaluateBashRule("UNKNOWN_VAR=evil safe_cmd")
	assert.False(t, matched)
}
