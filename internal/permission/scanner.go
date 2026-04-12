package permission

import (
	"fmt"
	"regexp"
	"strings"
)

// Threat represents a detected dangerous pattern in a command.
type Threat struct {
	Pattern     string // the regex pattern that matched
	Category    string // threat category (e.g. "destructive", "exfiltration")
	Description string // human-readable explanation
	Severity    string // "high", "medium", "low"
}

// ScanResult holds the outcome of scanning a command.
type ScanResult struct {
	Safe    bool     // true if no threats detected
	Threats []Threat // all detected threats
}

// Summary returns a one-line summary of detected threats.
func (r ScanResult) Summary() string {
	if r.Safe {
		return "no threats detected"
	}
	var parts []string
	for _, t := range r.Threats {
		parts = append(parts, fmt.Sprintf("[%s] %s", t.Severity, t.Description))
	}
	return strings.Join(parts, "; ")
}

// dangerPattern pairs a compiled regex with threat metadata.
type dangerPattern struct {
	re       *regexp.Regexp
	category string
	desc     string
	severity string
}

// patterns is the static list of dangerous command patterns.
// Each pattern targets a known class of risk from the reference docs.
var patterns []dangerPattern

func init() {
	raw := []struct {
		pattern  string
		category string
		desc     string
		severity string
	}{
		// --- Destructive operations ---
		{`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|--recursive)\b`, "destructive", "recursive force delete (rm -rf)", "high"},
		{`\brm\s+-[a-zA-Z]*f\b`, "destructive", "force delete (rm -f)", "medium"},
		{`\bmkfs\b`, "destructive", "filesystem format (mkfs)", "high"},
		{`\bdd\s+`, "destructive", "low-level disk write (dd)", "high"},
		{`>\s*/dev/sd[a-z]`, "destructive", "direct device write", "high"},

		// --- Shell config modification ---
		{`>\s*~/?\.(bashrc|bash_profile|zshrc|profile|zprofile|zshenv|zlogin)`, "persistence", "shell config overwrite", "high"},
		{`>>\s*~/?\.(bashrc|bash_profile|zshrc|profile|zprofile|zshenv|zlogin)`, "persistence", "shell config append", "high"},
		{`\bsource\s+~/?\.(bashrc|bash_profile|zshrc|profile)`, "execution", "shell config reload (may execute injected code)", "medium"},

		// --- Privilege escalation ---
		{`\bsudo\s+`, "privilege", "privilege escalation (sudo)", "medium"},
		{`\bsu\s+-?\s`, "privilege", "switch user (su)", "medium"},
		{`\bchmod\s+[0-7]*7[0-7]*\s`, "privilege", "world-writable permission (chmod)", "medium"},
		{`\bchmod\s+\+s\b`, "privilege", "setuid/setgid bit (chmod +s)", "high"},

		// --- Network exfiltration ---
		{`\bcurl\s+.*(-X\s*POST|--data|--upload-file|-d\s)`, "exfiltration", "data upload via curl", "medium"},
		{`\bwget\s+.*--post`, "exfiltration", "data upload via wget", "medium"},
		{`\bnc\s+-[a-zA-Z]*l`, "exfiltration", "netcat listener (reverse shell risk)", "high"},
		{`\bncat\s+`, "exfiltration", "ncat network utility", "medium"},

		// --- Pipe injection / redirection risks ---
		{`\|\s*sh\b`, "injection", "pipe to shell (command injection)", "high"},
		{`\|\s*bash\b`, "injection", "pipe to bash (command injection)", "high"},
		{`\|\s*zsh\b`, "injection", "pipe to zsh (command injection)", "high"},
		{`\beval\s+`, "injection", "eval command (code injection)", "high"},
		{`\$\(.*\).*\|`, "injection", "command substitution piped", "low"},

		// --- Git-sensitive operations ---
		{`\bgit\s+push\s+.*--force\b`, "destructive", "force push (git push --force)", "high"},
		{`\bgit\s+push\s+-f\b`, "destructive", "force push (git push -f)", "high"},
		{`\bgit\s+reset\s+--hard\b`, "destructive", "hard reset (git reset --hard)", "high"},
		{`\bgit\s+clean\s+-[a-zA-Z]*f`, "destructive", "git clean force", "medium"},

		// --- Sensitive file access ---
		{`\b(cat|less|more|head|tail)\s+.*(\.env|credentials|\.pem|\.key|id_rsa)`, "sensitive", "access to secret/key file", "medium"},
		{`>\s*\.(env|pem|key)\b`, "sensitive", "write to secret file", "high"},

		// --- Process/system manipulation ---
		{`\bkill\s+-9\b`, "system", "force kill process", "low"},
		{`\bkillall\b`, "system", "kill all matching processes", "medium"},
		{`\bpkill\b`, "system", "pattern-based process kill", "medium"},
		{`\bshutdown\b`, "system", "system shutdown", "high"},
		{`\breboot\b`, "system", "system reboot", "high"},

		// --- Claude/Glaude config tampering ---
		{`>\s*\.claude/`, "config", "write to .claude/ directory", "high"},
		{`>\s*\.glaude`, "config", "write to .glaude config", "high"},
		{`\brm\s+.*\.claude/`, "config", "delete .claude/ files", "high"},
		{`\brm\s+.*\.glaude`, "config", "delete .glaude config", "high"},
	}

	patterns = make([]dangerPattern, 0, len(raw))
	for _, r := range raw {
		patterns = append(patterns, dangerPattern{
			re:       regexp.MustCompile(r.pattern),
			category: r.category,
			desc:     r.desc,
			severity: r.severity,
		})
	}
}

// ScanCommand checks a bash command string against all known dangerous patterns.
func ScanCommand(cmd string) ScanResult {
	var threats []Threat
	for _, p := range patterns {
		if p.re.MatchString(cmd) {
			threats = append(threats, Threat{
				Pattern:     p.re.String(),
				Category:    p.category,
				Description: p.desc,
				Severity:    p.severity,
			})
		}
	}
	return ScanResult{
		Safe:    len(threats) == 0,
		Threats: threats,
	}
}

// HasHighSeverity returns true if any threat is high severity.
func (r ScanResult) HasHighSeverity() bool {
	for _, t := range r.Threats {
		if t.Severity == "high" {
			return true
		}
	}
	return false
}
