// Package ext is where a module adds to the panel without changing it (docs/editions.md). A module is a
// Go package outside this repository, such as the enterprise edition's, that calls Register from its
// init function; its binary imports it and then calls app.Main. The panel reads everything through this
// package, so with no module registered it is exactly the panel's own generated registry.
//
// A module generates its own registry the same way the panel does:
//
//	go run shippin.cloud/console/cmd/mcplgen -root . -module shippin.cloud/console/enterprise -out internal/registry/registry_gen.go -pkg registry
//
// and registers it:
//
//	func init() {
//		ext.Register(ext.Module{Name: "enterprise", Actions: registry.All, Reactions: registry.Reactions,
//			Providers: registry.Providers, Settings: registry.Settings})
//	}
package ext

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/settings"
)

// Module is what one module adds to the panel.
type Module struct {
	// Name identifies the module in logs and errors. It is unique.
	Name string
	// Actions, Reactions, Providers and Settings are the module's generated registry (cmd/mcplgen -module).
	Actions   []mcpl.Registered
	Reactions []mcpl.RegisteredReaction
	Providers []mcpl.Provider
	Settings  []settings.Section
	// Perms are the module's staff permissions, added to the panel's catalog. A permission's key must
	// not be the panel's or another module's.
	Perms []Perm
	// Jobs are the module's kinds of durable job, run by the panel's job queue.
	Jobs []Job
	// Caller is the module's caller policy (policy.go). At most one module sets it.
	Caller CallerPolicy
	// Migrations are the module's own *.sql files, at the root of this file system (an embed.FS
	// sub-directory, for instance). They run after the panel's, in name order, in the same lock and
	// migration table, recorded as module:<Name>/<file>. They may build on the panel's tables.
	Migrations fs.FS
}

// Perm is one staff permission: an entry of the catalog that roles pick from and every staff route
// names. The panel's own catalog is in internal/api.
type Perm struct {
	Key      string `json:"key"`
	Section  string `json:"section"`
	Label    string `json:"label"`
	Risk     bool   `json:"high_risk"`
	Reserved bool   `json:"reserved"` // no route yet; may be assigned, grants nothing today
	// Grantable permissions may also be granted to one principal directly (Team > Direct grants),
	// cloud-IAM style. Only the Owner preset holds them by default; every other role is denied.
	Grantable bool `json:"grantable"`
	// Presets are the keys of the preset staff roles (such as staff.owner) that hold this permission
	// on a fresh database. An existing installation's roles gain it through the module's migrations.
	Presets []string `json:"-"`
}

// Job is one kind of durable job. Handler is called once, when the panel's job queue starts, with what
// the panel runs jobs on.
type Job struct {
	Kind    string
	Handler func(Runtime) jobs.Handler
}

// Runtime is what the panel hands a module's job handlers: its database and the services its actions
// use (mcpl.Need).
type Runtime struct {
	Pool     *pgxpool.Pool
	Services mcpl.Services
}

var (
	mu      sync.Mutex
	modules []Module
)

// Register adds a module. Call it from an init function, before app.Main. It panics on a missing or
// repeated name, or on an action, reaction or settings section that is already declared, so a clash
// stops the binary at start rather than shadowing a route.
func Register(m Module) {
	mu.Lock()
	defer mu.Unlock()
	if m.Name == "" {
		panic("ext: a module needs a name")
	}
	for _, have := range modules {
		if have.Name == m.Name {
			panic(fmt.Sprintf("ext: module %q is registered twice", m.Name))
		}
	}
	next := append(slices.Clone(modules), m)
	if err := check(next); err != nil {
		panic("ext: module " + m.Name + ": " + err.Error())
	}
	modules = next
}

// Modules are the registered modules, in the order they registered.
func Modules() []Module {
	mu.Lock()
	defer mu.Unlock()
	return slices.Clone(modules)
}

// Actions are the panel's actions and every module's, in ID order.
func Actions() []mcpl.Registered {
	out := slices.Clone(actions.All)
	for _, m := range Modules() {
		out = append(out, m.Actions...)
	}
	slices.SortFunc(out, func(a, b mcpl.Registered) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Reactions are the panel's reactions and every module's, in ID order.
func Reactions() []mcpl.RegisteredReaction {
	out := slices.Clone(actions.Reactions)
	for _, m := range Modules() {
		out = append(out, m.Reactions...)
	}
	slices.SortFunc(out, func(a, b mcpl.RegisteredReaction) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Providers are the panel's entity services and every module's.
func Providers() []mcpl.Provider {
	out := slices.Clone(actions.Providers)
	for _, m := range Modules() {
		out = append(out, m.Providers...)
	}
	return out
}

// Perms are every module's staff permissions. internal/api adds them to the panel's catalog.
func Perms() []Perm {
	var out []Perm
	for _, m := range Modules() {
		out = append(out, m.Perms...)
	}
	return out
}

// Jobs are every module's kinds of job. internal/api registers them with the panel's job queue.
func Jobs() []Job {
	var out []Job
	for _, m := range Modules() {
		out = append(out, m.Jobs...)
	}
	return out
}

// Migration is one module's migration files.
type Migration struct {
	Name string
	FS   fs.FS
}

// Migrations are the migration files of every module that has them, in registration order.
func Migrations() []Migration {
	var out []Migration
	for _, m := range Modules() {
		if m.Migrations != nil {
			out = append(out, Migration{Name: m.Name, FS: m.Migrations})
		}
	}
	return out
}

// Settings are the panel's settings sections and every module's.
func Settings() []settings.Section {
	out := slices.Clone(actions.Settings)
	for _, m := range Modules() {
		out = append(out, m.Settings...)
	}
	return out
}

// Swap replaces the registered modules and returns a function that restores them. It is for tests that
// prove a module plugs in; a binary registers its modules once, with Register.
func Swap(ms ...Module) (restore func()) {
	mu.Lock()
	defer mu.Unlock()
	if err := check(ms); err != nil {
		panic("ext: " + err.Error())
	}
	old := modules
	modules = slices.Clone(ms)
	return func() {
		mu.Lock()
		defer mu.Unlock()
		modules = old
	}
}

// check refuses an action, reaction or settings section declared twice across the panel and ms, and a
// permission or job kind declared twice across ms (internal/api compares them with the panel's own).
func check(ms []Module) error {
	ids := map[string]string{}
	claim := func(kind, id, owner string) error {
		if prev, ok := ids[kind+" "+id]; ok {
			return fmt.Errorf("%s %s is declared by both %s and %s", kind, id, prev, owner)
		}
		ids[kind+" "+id] = owner
		return nil
	}
	for _, a := range actions.All {
		_ = claim("action", a.ID, "the panel")
	}
	for _, r := range actions.Reactions {
		_ = claim("reaction", r.ID, "the panel")
	}
	for _, s := range actions.Settings {
		_ = claim("settings section", s.Key, "the panel")
	}
	for _, m := range ms {
		for _, a := range m.Actions {
			if err := claim("action", a.ID, "module "+m.Name); err != nil {
				return err
			}
		}
		for _, r := range m.Reactions {
			if err := claim("reaction", r.ID, "module "+m.Name); err != nil {
				return err
			}
		}
		for _, s := range m.Settings {
			if err := claim("settings section", s.Key, "module "+m.Name); err != nil {
				return err
			}
		}
		for _, p := range m.Perms {
			if p.Key == "" {
				return fmt.Errorf("module %s has a permission without a key", m.Name)
			}
			if err := claim("permission", p.Key, "module "+m.Name); err != nil {
				return err
			}
		}
		if m.Caller != nil {
			if err := claim("policy", "caller", "module "+m.Name); err != nil {
				return err
			}
		}
		for _, j := range m.Jobs {
			if j.Kind == "" || j.Handler == nil {
				return fmt.Errorf("module %s has a job without a kind or a handler", m.Name)
			}
			if err := claim("job kind", j.Kind, "module "+m.Name); err != nil {
				return err
			}
		}
	}
	return nil
}
