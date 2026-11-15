// Package runner_size is the runner sizes staff offer for customers' build jobs (RUN-01,
// docs/runners.md). A job asks for one with `runs-on: shippin-<id>`; a plan says which sizes its
// organizations may use (the runners service in the plan matrix). Staff with catalog.plans.write
// change sizes; staff with catalog.plans.read read them.
package runner_size

import (
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"shippin.cloud/console/internal/mcpl"
)

// Size is one runner size: the machine each job that asks for it gets.
type Size struct {
	ID string `json:"id"`
	// Label is what a customer reads, such as "2 cores, 8 GB".
	Label     string `json:"label"`
	Cores     int    `json:"cores"`
	MemoryMiB int    `json:"memory_mib"`
	DiskGiB   int    `json:"disk_gib"`
	// Image is the runner image on the provider; while it is empty no job can start on this size.
	Image     string    `json:"image"`
	Active    bool      `json:"active"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Fields are what create and update take. Active defaults to true.
type Fields struct {
	Label     string `json:"label"`
	Cores     int    `json:"cores"`
	MemoryMiB int    `json:"memory_mib"`
	DiskGiB   int    `json:"disk_gib"`
	Image     string `json:"image"`
	Active    *bool  `json:"active,omitempty"`
}

// ByID names one size from the route.
type ByID struct {
	ID string `path:"runnerSizeId" json:"-"`
}

// Prefix starts the runs-on label of every Shippin size: runs-on: shippin-2x asks for size 2x.
const Prefix = "shippin-"

var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)

func (f Fields) check(id string) error {
	switch {
	case !slug.MatchString(id):
		return mcpl.Invalid("A runner size needs a short lowercase id, such as 2x or large; jobs ask for it as shippin-<id>.")
	case f.Label == "" || len(f.Label) > 80:
		return mcpl.Invalid("Give the size a label of up to 80 characters that says what a customer gets.")
	case f.Cores < 1 || f.Cores > 128 || f.MemoryMiB < 512 || f.MemoryMiB > 1048576 || f.DiskGiB < 8 || f.DiskGiB > 4096:
		return mcpl.Invalid("A runner needs 1 to 128 cores, 512 MiB to 1 TiB of memory and 8 GiB to 4 TiB of disk.")
	case len(f.Image) > 200:
		return mcpl.Invalid("The image name is at most 200 characters.")
	}
	return nil
}

func active(b *bool) bool { return b == nil || *b }

const cols = `id, label, cores, memory_mib, disk_gib, image, active, updated_at`

func scan(row pgx.Row) (Size, error) {
	var s Size
	err := row.Scan(&s.ID, &s.Label, &s.Cores, &s.MemoryMiB, &s.DiskGiB, &s.Image, &s.Active, &s.UpdatedAt)
	return s, err
}

// Find is the size with this id, for the job intake; ok is false when there is none.
func Find(c *mcpl.Call, id string) (s Size, ok bool, err error) {
	s, err = scan(c.DB.QueryRow(c.Ctx, `SELECT `+cols+` FROM shippin_runner_sizes WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return s, false, nil
	}
	return s, err == nil, err
}

func one(c *mcpl.Call, id string) (Size, error) {
	s, ok, err := Find(c, id)
	if err == nil && !ok {
		return s, mcpl.NotFound("No runner size " + id + ".")
	}
	return s, err
}

func taken(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
