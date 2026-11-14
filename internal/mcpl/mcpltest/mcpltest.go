// Package mcpltest serves generated actions on their derived routes for an entity's tests, against a
// fresh test database, and collects their audit entries and events.
package mcpltest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/mcpl"
)

// As is who the actions run as.
type As struct {
	Actor  string
	Tenant string
	// Perms answers c.Can; nil allows nothing extra.
	Perms    func(perm string) bool
	Services mcpl.Services
}

// Harness is the actions whose IDs start with a prefix, served as one caller.
type Harness struct {
	t      *testing.T
	Pool   *pgxpool.Pool
	Mux    *http.ServeMux
	As     As
	Audits []string
	Events []mcpl.Event
}

// DB is a migrated, empty test database; the test is skipped when SHIPPIN_TEST_DATABASE_URL is unset.
func DB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	d.Pool.Close()
	if d, err = db.Open(ctx, url); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	return d.Pool
}

// Serve mounts the actions in all whose ID starts with prefix on a fresh database.
func Serve(t *testing.T, all []mcpl.Registered, prefix string, as As) *Harness {
	t.Helper()
	h := &Harness{t: t, Pool: DB(t), Mux: http.NewServeMux(), As: as}
	for _, g := range all {
		if !strings.HasPrefix(g.ID, prefix) {
			continue
		}
		h.Mux.HandleFunc(g.Route, func(w http.ResponseWriter, r *http.Request) {
			g.Serve(&mcpl.Call{Ctx: r.Context(), Pool: h.Pool, Actor: h.As.Actor, Tenant: h.As.Tenant, Perms: h.As.Perms, Services: h.As.Services,
				Audit: func(_ mcpl.DB, action, detail string) error {
					h.Audits = append(h.Audits, action+" "+detail)
					return nil
				},
				Publish: func(e mcpl.Event) { h.Events = append(h.Events, e) }}, w, r)
		})
	}
	return h
}

// Call runs one request and fails the test unless it answers want; it returns the decoded JSON body.
func (h *Harness) Call(method, path, body string, want int) map[string]any {
	h.t.Helper()
	rec := httptest.NewRecorder()
	h.Mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	if rec.Code != want {
		h.t.Fatalf("%s %s: got %d want %d: %s", method, path, rec.Code, want, rec.Body.String())
	}
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}
