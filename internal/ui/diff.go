package ui

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// RenderDiff computes and renders a colorized unified diff between old and new content.
// The path parameter is used in the diff header.
func RenderDiff(path, oldContent, newContent string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(oldContent, newContent, true)

	if len(diffs) == 1 && diffs[0].Type == diffmatchpatch.DiffEqual {
		return "" // no changes
	}

	var b strings.Builder
	b.WriteString(diffHeaderStyle.Render(fmt.Sprintf("--- a/%s", path)))
	b.WriteString("\n")
	b.WriteString(diffHeaderStyle.Render(fmt.Sprintf("+++ b/%s", path)))
	b.WriteString("\n")

	// Render line-level diff
	patches := dmp.PatchMake(diffs)
	for _, patch := range patches {
		b.WriteString(diffHunkStyle.Render(patch.String()))
	}

	// If patches are empty, fall back to inline diff display
	if len(patches) == 0 {
		for _, diff := range diffs {
			lines := strings.Split(diff.Text, "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}
				switch diff.Type {
				case diffmatchpatch.DiffInsert:
					b.WriteString(diffAddStyle.Render("+ " + line))
					b.WriteString("\n")
				case diffmatchpatch.DiffDelete:
					b.WriteString(diffDelStyle.Render("- " + line))
					b.WriteString("\n")
				case diffmatchpatch.DiffEqual:
					// skip equal lines for brevity
				}
			}
		}
	}

	return b.String()
}

// RenderUnifiedDiff produces a simpler line-based unified diff output.
func RenderUnifiedDiff(path, oldContent, newContent string) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	dmp := diffmatchpatch.New()
	// Use line-mode diff for better readability
	a, b2, lineArray := dmp.DiffLinesToChars(oldContent, newContent)
	diffs := dmp.DiffMain(a, b2, false)
	dmp.DiffCharsToLines(diffs, lineArray)

	if len(diffs) == 1 && diffs[0].Type == diffmatchpatch.DiffEqual {
		return "" // no changes
	}

	var sb strings.Builder
	sb.WriteString(diffHeaderStyle.Render(fmt.Sprintf("--- a/%s (%d lines)", path, len(oldLines))))
	sb.WriteString("\n")
	sb.WriteString(diffHeaderStyle.Render(fmt.Sprintf("+++ b/%s (%d lines)", path, len(newLines))))
	sb.WriteString("\n")

	for _, diff := range diffs {
		lines := strings.Split(strings.TrimRight(diff.Text, "\n"), "\n")
		for _, line := range lines {
			switch diff.Type {
			case diffmatchpatch.DiffInsert:
				sb.WriteString(diffAddStyle.Render("+ " + line))
				sb.WriteString("\n")
			case diffmatchpatch.DiffDelete:
				sb.WriteString(diffDelStyle.Render("- " + line))
				sb.WriteString("\n")
			case diffmatchpatch.DiffEqual:
				// Show context lines (up to 3) around changes
				if len(lines) <= 6 {
					sb.WriteString("  " + line + "\n")
				}
			}
		}
	}

	return sb.String()
}
