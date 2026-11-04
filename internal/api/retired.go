package api

import "strings"

// RetiredNotice tells a site that still has a removed feature's settings what became of its data.
// The panel never reads those files, so this is only ever a message.
func RetiredNotice(getenv func(string) string) string {
	dirs := []string{}
	for _, k := range []string{"GOLDSIGN_DIR", "GOLDSIGN_FACTS_DIR"} {
		if d := strings.TrimSpace(getenv(k)); d != "" {
			dirs = append(dirs, d)
		}
	}
	if len(dirs) == 0 {
		return ""
	}
	return "change signing has been removed from this panel; its records in " + strings.Join(dirs, " and ") + " are untouched: see docs/approval.md"
}
