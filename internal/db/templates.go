package db

import (
	"context"
	"time"
)

// Template is a workspace blueprint: image + toolchain + default size + optional repo/startup/ports.
type Template struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	Image       string    `json:"image"`
	Toolchain   []string  `json:"toolchain"`
	CPU         int       `json:"cpu"`
	MemoryGiB   int       `json:"memory_gib"`
	DiskGiB     int       `json:"disk_gib"`
	Repo        string    `json:"repo"`
	Startup     string    `json:"startup"`
	Ports       []int     `json:"ports"`
	Enabled     bool      `json:"enabled"`
	Sort        int       `json:"sort"`
	CreatedAt   time.Time `json:"created_at"`
}

const tplCols = `id::text, slug, name, description, icon, image, toolchain, cpu, memory_gib, disk_gib, repo, startup, ports, enabled, sort, created_at`

func (d *DB) Templates(ctx context.Context, enabledOnly bool) ([]Template, error) {
	where := ""
	if enabledOnly {
		where = "WHERE enabled"
	}
	rows, err := d.Pool.Query(ctx, `SELECT `+tplCols+` FROM templates `+where+` ORDER BY sort, name`)
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
	return out, nil
}

func (d *DB) Template(ctx context.Context, id string) (*Template, error) {
	ts, err := d.Templates(ctx, false)
	if err != nil {
		return nil, err
	}
	for i := range ts {
		if ts[i].ID == id || ts[i].Slug == id {
			return &ts[i], nil
		}
	}
	return nil, nil
}

// SaveTemplate inserts or updates by id; insertOnly skips existing slugs (seeding).
func (d *DB) SaveTemplate(ctx context.Context, t Template, insertOnly bool) error {
	conflict := `ON CONFLICT (id) DO UPDATE SET slug=$2, name=$3, description=$4, icon=$5, image=$6, toolchain=$7, cpu=$8,
		memory_gib=$9, disk_gib=$10, repo=$11, startup=$12, ports=$13, enabled=$14, sort=$15, updated_at=now()`
	if insertOnly {
		conflict = `ON CONFLICT (slug) DO NOTHING`
	}
	_, err := d.Pool.Exec(ctx, `INSERT INTO templates(`+
		`id, slug, name, description, icon, image, toolchain, cpu, memory_gib, disk_gib, repo, startup, ports, enabled, sort)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) `+conflict,
		t.ID, t.Slug, t.Name, t.Description, t.Icon, t.Image, t.Toolchain, t.CPU, t.MemoryGiB, t.DiskGiB,
		t.Repo, t.Startup, t.Ports, t.Enabled, t.Sort)
	return err
}

func (d *DB) DeleteTemplate(ctx context.Context, id string) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM templates WHERE id::text=$1`, id)
	return err
}
