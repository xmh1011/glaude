package bundled

import "github.com/xmh1011/glaude/internal/skill"

const commitPrompt = `Generate a git commit for the current staged changes. Follow these steps:

1. Run ` + "`git status`" + ` to see all untracked and modified files.
2. Run ` + "`git diff --cached`" + ` to see staged changes. If nothing is staged, run ` + "`git diff`" + ` to see unstaged changes.
3. Run ` + "`git log --oneline -5`" + ` to see recent commit message style.
4. Analyze the changes and draft a commit message:
   - Use the conventional commit format: ` + "`<type>(<scope>): <description>`" + `
   - Types: feat, fix, refactor, docs, test, chore, style, perf, ci, build
   - Keep the first line under 72 characters
   - Add a body if the change is non-trivial, explaining the "why" not the "what"
5. Stage relevant files if needed (prefer specific files over ` + "`git add -A`" + `)
6. Create the commit

Do NOT push to remote unless explicitly asked.
Do NOT commit files that likely contain secrets (.env, credentials, etc).`

func commitSkill() *skill.Skill {
	return &skill.Skill{
		Name:          "commit",
		Description:   "Generate and create a conventional commit for current changes",
		WhenToUse:     "When the user wants to commit their current changes with a well-formatted message",
		UserInvocable: true,
		Source:         "bundled",
		GetPrompt: func(args string) (string, error) {
			if args != "" {
				return commitPrompt + "\n\nAdditional instructions: " + args, nil
			}
			return commitPrompt, nil
		},
	}
}
