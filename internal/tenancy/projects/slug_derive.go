package projects

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"
)

var slugJunk = regexp.MustCompile(`[^a-z0-9]+`)

// SlugFromSource derives a slug from a repo full name ("owner/My_Repo.git") or a folder path
// ("/home/dev/code/My App"): last path element, lowercased, non-alphanumerics -> '-', trimmed to
// 30. A leading digit gets a "p-" prefix; a reserved or too-short result gets a "-app" suffix.
func SlugFromSource(src string) string {
	b := strings.TrimSuffix(path.Base(strings.TrimRight(src, "/")), ".git")
	s := strings.Trim(slugJunk.ReplaceAllString(strings.ToLower(b), "-"), "-")
	if s != "" && s[0] >= '0' && s[0] <= '9' {
		s = "p-" + s
	}
	if len(s) > 30 {
		s = strings.TrimRight(s[:30], "-")
	}
	if ValidSlug(s) != nil {
		s = strings.TrimRight(s[:min(len(s), 26)], "-") + "-app"
		s = strings.TrimLeft(s, "-")
	}
	if ValidSlug(s) != nil {
		return "project"
	}
	return s
}

// UniqueSlug returns base, or base-2 … base-50, the first that taken reports free in the org.
func UniqueSlug(ctx context.Context, base string, taken func(context.Context, string) (bool, error)) (string, error) {
	for i := 1; i <= 50; i++ {
		s := base
		if i > 1 {
			suf := fmt.Sprintf("-%d", i)
			s = strings.TrimRight(base[:min(len(base), 30-len(suf))], "-") + suf
		}
		t, err := taken(ctx, s)
		if err != nil {
			return "", err
		}
		if !t {
			return s, nil
		}
	}
	return "", ErrNameTaken
}

// SlugTaken reports whether slug is used by a live project of the org that tenant maps to.
func (s *Store) SlugTaken(ctx context.Context, tenant, slug string) (bool, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM projects p JOIN tenants t ON t.idp_org_id = p.org_id
	  WHERE t.id=$1 AND p.slug=$2 AND p.state <> 'deleting'`, tenant, slug).Scan(&n)
	return n > 0, err
}

// AdoptedPaths maps devspace folders already backing a live project of the tenant to the project id.
func (s *Store) AdoptedPaths(ctx context.Context, tenant string) (map[string]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT source_path, id::text FROM projects WHERE tenant_id=$1 AND source_path IS NOT NULL AND state <> 'deleting'`, tenant)
	if err != nil {
		return map[string]string{}, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var p, id string
		if rows.Scan(&p, &id) == nil {
			out[p] = id
		}
	}
	return out, rows.Err()
}
