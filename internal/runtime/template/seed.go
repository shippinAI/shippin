package template

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/platform/config"
)

// Seed adds the templates in the configured seed file that the database does not have yet, by slug.
// One that fails Check is logged and left out.
func Seed(ctx context.Context, q mcpl.DB, c config.Workspaces) {
	if c.TemplateSeed == "" {
		return
	}
	raw, err := os.ReadFile(c.TemplateSeed)
	if err != nil {
		log.Printf("template seed: %v", err)
		return
	}
	ts, errs := parseSeed(raw, c)
	for _, e := range errs {
		log.Printf("template seed: %v", e)
	}
	for _, t := range ts {
		if err := insert(ctx, q, t, `ON CONFLICT (slug) DO NOTHING`); err != nil {
			log.Printf("template seed %s: %v", t.Slug, err)
		}
	}
}

// parseSeed decodes a seed file and keeps only the templates that pass Check, each with an id.
func parseSeed(raw []byte, c config.Workspaces) ([]Template, []error) {
	var in []Template
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, []error{err}
	}
	out, errs, seen := []Template{}, []error{}, map[string]bool{}
	for _, t := range in {
		if msg := Check(&t, c); msg != "" {
			errs = append(errs, fmt.Errorf("%q skipped: %s", t.Slug, msg))
			continue
		}
		if seen[t.Slug] {
			errs = append(errs, fmt.Errorf("%q skipped: duplicate slug", t.Slug))
			continue
		}
		seen[t.Slug] = true
		if uuid.Validate(t.ID) != nil {
			id, _ := uuid.NewV7()
			t.ID = id.String()
		}
		out = append(out, t)
	}
	return out, errs
}
