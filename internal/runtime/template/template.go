// Package template is what a development space starts from: an image, a size, a repository, a start
// command and the ports it serves. This file is its type and its storage.
package template

import (
	"context"
	"time"

	"shippin.cloud/console/internal/mcpl"
)

// Template is what a development space starts from.
type Template struct {
	ID string `json:"id"`
	Fields
	CreatedAt time.Time `json:"created_at"`
}

// Fields are what staff write: the image, the tools it lists, its default size, and optionally a
// repository to clone, a start command and the ports it serves.
type Fields struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Image       string   `json:"image"`
	Toolchain   []string `json:"toolchain"`
	CPU         int      `json:"cpu"`
	MemoryGiB   int      `json:"memory_gib"`
	DiskGiB     int      `json:"disk_gib"`
	Repo        string   `json:"repo"`
	Startup     string   `json:"startup"`
	Ports       []int    `json:"ports"`
	Enabled     bool     `json:"enabled"`
	Sort        int      `json:"sort"`
}

// ByID names one template from the route.
type ByID struct {
	ID string `path:"templateId" json:"-"`
}

const tplCols = `id::text, slug, name, description, icon, image, toolchain, cpu, memory_gib, disk_gib, repo, startup, ports, enabled, sort, created_at`

// all lists the templates in the order the create page shows them.
func all(ctx context.Context, q mcpl.DB, enabledOnly bool) ([]Template, error) {
	where := ""
	if enabledOnly {
		where = "WHERE enabled"
	}
	rows, err := q.Query(ctx, `SELECT `+tplCols+` FROM templates `+where+` ORDER BY sort, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Template{}
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Description, &t.Icon, &t.Image, &t.Toolchain, &t.CPU,
			&t.MemoryGiB, &t.DiskGiB, &t.Repo, &t.Startup, &t.Ports, &t.Enabled, &t.Sort, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Enabled are the templates a person may pick, in the order the create page shows them.
func Enabled(ctx context.Context, q mcpl.DB) ([]Template, error) { return all(ctx, q, true) }

// Find is the template with this id or slug, or nil.
func Find(ctx context.Context, q mcpl.DB, idOrSlug string) (*Template, error) {
	ts, err := all(ctx, q, false)
	if err != nil {
		return nil, err
	}
	for i := range ts {
		if ts[i].ID == idOrSlug || ts[i].Slug == idOrSlug {
			return &ts[i], nil
		}
	}
	return nil, nil
}

// one is the template with this id, or a 404.
func one(c *mcpl.Call, id string) (Template, error) {
	ts, err := all(c.Ctx, c.DB, false)
	if err != nil {
		return Template{}, err
	}
	for _, t := range ts {
		if t.ID == id {
			return t, nil
		}
	}
	return Template{}, mcpl.NotFound("No template " + id + ".")
}

const insertSQL = `INSERT INTO templates(id, slug, name, description, icon, image, toolchain, cpu, memory_gib, disk_gib, repo, startup, ports, enabled, sort)
  VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`

func insert(ctx context.Context, q mcpl.DB, t Template, onConflict ...string) error {
	sql := insertSQL
	for _, c := range onConflict {
		sql += " " + c
	}
	_, err := q.Exec(ctx, sql, t.ID, t.Slug, t.Name, t.Description, t.Icon, t.Image, t.Toolchain, t.CPU, t.MemoryGiB, t.DiskGiB,
		t.Repo, t.Startup, t.Ports, t.Enabled, t.Sort)
	return err
}
