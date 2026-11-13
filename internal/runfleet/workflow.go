package runfleet

import (
	"fmt"
	"strings"
)

// WorkflowFile is one Actions workflow as text.
type WorkflowFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// FileChange is a proposed write. Before is empty when the file is new.
type FileChange struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Before  string `json:"before,omitempty"`
}

// WorkflowPlan is a dry-run unless a pull request URL is set.
type WorkflowPlan struct {
	DryRun   bool         `json:"dry_run"`
	Changes  []FileChange `json:"changes"`
	Diff     string       `json:"diff"`
	PullURL  string       `json:"pull_url,omitempty"`
	Warnings []string     `json:"warnings,omitempty"`
}

// LovableWorkflow is the file added when a Lovable-synced repo has no workflow yet.
func LovableWorkflow(label string) WorkflowFile {
	return WorkflowFile{
		Path: ".github/workflows/shippin-run.yml",
		Content: fmt.Sprintf(`name: shippin-run
on: [push, pull_request]
jobs:
  build:
    runs-on: %s
    steps:
      # Pin actions/checkout to a commit before relying on this in a public repo.
      - uses: actions/checkout@v4
      - run: echo "Replace this step with the project build."
`, label),
	}
}

// PlanRunsOn rewrites scalar and list runs-on values to label.
// Expressions such as ${{ matrix.os }} are left in place and reported.
func PlanRunsOn(files []WorkflowFile, label string, lovable, missingWorkflow bool) WorkflowPlan {
	plan := WorkflowPlan{DryRun: true}
	if missingWorkflow && lovable {
		file := LovableWorkflow(label)
		plan.Changes = append(plan.Changes, FileChange{Path: file.Path, Content: file.Content})
		plan.Diff = unified(file.Path, "", file.Content)
		return plan
	}
	var diffs []string
	replaced := 0
	for _, file := range files {
		next, n, warnings := patchRunsOn(file.Content, label)
		plan.Warnings = append(plan.Warnings, warnings...)
		replaced += n
		if next == file.Content {
			continue
		}
		plan.Changes = append(plan.Changes, FileChange{Path: file.Path, Content: next, Before: file.Content})
		diffs = append(diffs, unified(file.Path, file.Content, next))
	}
	plan.Diff = strings.Join(diffs, "")
	if len(files) == 0 {
		plan.Warnings = append(plan.Warnings, "repository has no workflow file to patch")
	} else if replaced == 0 && len(plan.Changes) == 0 {
		plan.Warnings = append(plan.Warnings, "no runs-on value was replaced")
	}
	return plan
}

func patchRunsOn(content, label string) (string, int, []string) {
	lines := strings.Split(content, "\n")
	var warnings []string
	n := 0
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		space, value, ok := splitRunsOn(line)
		if !ok {
			out = append(out, line)
			continue
		}
		if strings.TrimSpace(value) == "" {
			j := i + 1
			list := false
			for j < len(lines) {
				next := lines[j]
				if strings.TrimSpace(next) == "" {
					j++
					continue
				}
				if indentOf(next) > len(space) && strings.HasPrefix(strings.TrimSpace(next), "- ") {
					list = true
					j++
					continue
				}
				break
			}
			if !list {
				warnings = append(warnings, "runs-on has no value and was left unchanged")
				out = append(out, line)
				continue
			}
			out = append(out, space+"runs-on: "+label)
			n++
			i = j - 1
			continue
		}
		if strings.Contains(value, "${{") {
			warnings = append(warnings, "left an expression runs-on unchanged")
			out = append(out, line)
			continue
		}
		out = append(out, space+"runs-on: "+label)
		n++
	}
	trailing := ""
	if strings.HasSuffix(content, "\n") {
		trailing = "\n"
	}
	text := strings.Join(out, "\n")
	if trailing != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text, n, warnings
}

func splitRunsOn(line string) (space, value string, ok bool) {
	trim := strings.TrimLeft(line, " \t")
	space = line[:len(line)-len(trim)]
	const key = "runs-on:"
	if !strings.HasPrefix(trim, key) {
		return "", "", false
	}
	return space, strings.TrimSpace(trim[len(key):]), true
}

func indentOf(line string) int {
	n := 0
	for _, r := range line {
		if r == ' ' || r == '\t' {
			n++
			continue
		}
		break
	}
	return n
}

func unified(path, before, after string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n", path, path)
	if before == "" {
		for _, line := range strings.Split(strings.TrimRight(after, "\n"), "\n") {
			fmt.Fprintf(&b, "+%s\n", line)
		}
		return b.String()
	}
	beforeLines := strings.Split(strings.TrimRight(before, "\n"), "\n")
	afterLines := strings.Split(strings.TrimRight(after, "\n"), "\n")
	max := len(beforeLines)
	if len(afterLines) > max {
		max = len(afterLines)
	}
	for i := 0; i < max; i++ {
		var old, neu string
		if i < len(beforeLines) {
			old = beforeLines[i]
		}
		if i < len(afterLines) {
			neu = afterLines[i]
		}
		if i < len(beforeLines) && i < len(afterLines) && old == neu {
			fmt.Fprintf(&b, " %s\n", old)
			continue
		}
		if i < len(beforeLines) {
			fmt.Fprintf(&b, "-%s\n", old)
		}
		if i < len(afterLines) {
			fmt.Fprintf(&b, "+%s\n", neu)
		}
	}
	return b.String()
}
