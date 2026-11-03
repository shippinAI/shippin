package projects

import (
	"regexp"
	"strings"

	"shippin.cloud/console/internal/config"
)

var wordSplit = regexp.MustCompile(`[^a-z0-9.+#]+`)

// GuessStack picks a template slug for a prompt (implementation-plan-3 M4 "stack_guess"): the first
// configured stack whose keyword appears as a word (or phrase) in the prompt, else def.
// Deterministic: the table comes from config, nothing is inferred by a model.
func GuessStack(prompt string, stacks []config.Stack, def string) string {
	low := strings.ToLower(prompt)
	words := map[string]bool{}
	for _, w := range wordSplit.Split(low, -1) {
		words[strings.Trim(w, ".")] = true
	}
	padded := " " + strings.Join(wordSplit.Split(low, -1), " ") + " "
	for _, s := range stacks {
		for _, k := range s.Keywords {
			k = strings.ToLower(strings.TrimSpace(k))
			if k == "" {
				continue
			}
			if strings.Contains(k, " ") {
				if strings.Contains(padded, " "+k+" ") {
					return s.Template
				}
			} else if words[k] {
				return s.Template
			}
		}
	}
	return def
}
