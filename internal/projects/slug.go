package projects

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var (
	nonSlug   = regexp.MustCompile(`[^a-z0-9]+`)
	nameShape = regexp.MustCompile(`^[a-z][a-z0-9-]{0,38}[a-z0-9]$`)
	// filler words dropped from the start of a prompt when naming the project.
	filler = map[string]bool{"a": true, "an": true, "the": true, "build": true, "make": true, "create": true, "me": true,
		"please": true, "i": true, "want": true, "need": true, "simple": true, "small": true, "with": true, "using": true,
		"for": true, "of": true, "my": true, "and": true, "app": false}
)

// SlugFromPrompt turns a prompt into a project name (lower-kebab, ≤30, the 013 name shape):
// "Build me a todo app with Vite + React" → "todo-app-vite-react".
func SlugFromPrompt(prompt string) string {
	words := []string{}
	for _, w := range strings.Fields(nonSlug.ReplaceAllString(strings.ToLower(prompt), " ")) {
		if filler[w] {
			continue
		}
		words = append(words, w)
	}
	s := ""
	for _, w := range words {
		next := strings.Trim(s+"-"+w, "-")
		if len(next) > 30 {
			break
		}
		s = next
	}
	s = strings.TrimLeft(s, "0123456789-")
	if len(s) < 2 || !nameShape.MatchString(s) {
		return "project"
	}
	return s
}

// UniqueName returns base, or base-2, base-3… until taken reports false (≤40 chars).
func UniqueName(ctx context.Context, base string, taken func(context.Context, string) (bool, error)) (string, error) {
	for i := 1; i <= 50; i++ {
		n := base
		if i > 1 {
			suf := fmt.Sprintf("-%d", i)
			n = strings.TrimRight(base[:min(len(base), 40-len(suf))], "-") + suf
		}
		t, err := taken(ctx, n)
		if err != nil {
			return "", err
		}
		if !t {
			return n, nil
		}
	}
	return "", ErrNameTaken
}

// NameTaken reports whether tenant already has a project called name.
func (s *Store) NameTaken(ctx context.Context, tenant, name string) (bool, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM projects WHERE tenant_id=$1 AND name=$2`, tenant, name).Scan(&n)
	return n > 0, err
}
