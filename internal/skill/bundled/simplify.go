package bundled

import "github.com/xmh1011/glaude/internal/skill"

const simplifyPrompt = `You are performing a code review and cleanup of recent changes. Follow these phases:

## Phase 1: Identify Changes
Run ` + "`git diff`" + ` to identify all recent changes. If there are no staged/unstaged changes, run ` + "`git diff HEAD~1`" + ` to review the last commit.

## Phase 2: Three-Direction Review

Review the changes from three perspectives:

### 1. Code Reuse Review
- Are there existing utilities or helpers in the codebase that could replace new code?
- Is there any duplication with existing functionality?
- Could any new code be consolidated with similar existing patterns?

### 2. Code Quality Review
- Is there redundant state or unnecessary complexity?
- Are there parameter sprawl issues (too many function arguments)?
- Is there copy-paste code that should be abstracted?
- Are there leaky abstractions or unnecessary indirection layers?
- Are there comments that merely restate the code?

### 3. Efficiency Review
- Is there unnecessary work being done (extra allocations, redundant loops)?
- Are there missed opportunities for early returns or short-circuits?
- Could any hot paths be optimized?
- Are there potential memory leaks or resource cleanup issues?

## Phase 3: Fix Issues

For each issue found:
1. Explain what you found and why it matters
2. Apply the fix directly
3. Keep changes minimal and focused — do not refactor beyond what is needed

If no meaningful issues are found, say so and do not make changes for the sake of making changes.`

func simplifySkill() *skill.Skill {
	return &skill.Skill{
		Name:          "simplify",
		Description:   "Review and simplify recent code changes",
		WhenToUse:     "When the user wants to review, clean up, or simplify their recent code changes",
		UserInvocable: true,
		Source:         "bundled",
		GetPrompt: func(args string) (string, error) {
			if args != "" {
				return simplifyPrompt + "\n\nFocus on: " + args, nil
			}
			return simplifyPrompt, nil
		},
	}
}
