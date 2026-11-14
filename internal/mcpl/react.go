package mcpl

import (
	"fmt"
	"slices"
	"strings"
)

// Reaction is work an entity does when an event happens: push a person's SSH keys to their running
// workspaces after they add one. It is declared in the entity's folder, in a file named for it:
//
//	// internal/identity/ssh_key/push_to_workspaces.go
//	var PushToWorkspaces = mcpl.Reaction{
//		On:      []string{"shippin.identity.ssh_key.created", "shippin.identity.ssh_key.deleted"},
//		Summary: "Give the person's workspaces their current keys.",
//		Run:     func(c *mcpl.Call, e mcpl.Event) error { ... },
//	}
//
// The generator finds it, so adding the file is the whole change; it refuses an event that no action
// emits. Run gets a Call for the event's actor and organization, with the database and services, and
// runs after the change is committed, in its own goroutine. An error is logged, not returned to anyone.
type Reaction struct {
	// On is the events it follows. Each must be emitted by an action: a change's ID in the past tense.
	On      []string
	Summary string
	Run     func(c *Call, e Event) error
}

// RegisteredReaction is a reaction under the ID its location gives it:
// shippin.<domain>.<entity...>.<file name>.
type RegisteredReaction struct {
	ID string
	Reaction
}

// React registers r under id; the generated registry calls it.
func React(id string, r Reaction) RegisteredReaction { return RegisteredReaction{ID: id, Reaction: r} }

// Provider is an entity's own services: the implementations its actions Need that it can build from
// its own package (a Postgres repository, an HTTP client). It is declared once per entity package, as
//
//	var Services = mcpl.Provider(func(s mcpl.Services) { mcpl.Provide[Repository](s, PostgresRepository{}) })
//
// and the generator lists it. What only the running panel has (its sealer, the workspace runtime) is
// not an entity's to build: those are the host ports in internal/ports, provided once by the panel.
type Provider func(Services)

// ValidateReactions checks reactions against the actions: a well-formed ID unique among actions and
// reactions, a summary, a Run, and only events that a registered change emits.
func ValidateReactions(rs []RegisteredReaction, actions []Registered) []error {
	events := map[string]bool{}
	ids := map[string]bool{}
	for _, a := range actions {
		ids[a.ID] = true
		if a.Event != "" {
			events[a.Event] = true
		}
	}
	var errs []error
	fail := func(r RegisteredReaction, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s", r.ID, fmt.Sprintf(format, args...)))
	}
	for _, r := range rs {
		if !idPattern.MatchString(r.ID) {
			fail(r, "ID must be shippin.<domain>.<entity>.<name> in snake_case")
		} else if !slices.Contains(Domains, Action{ID: r.ID}.Domain()) {
			fail(r, "unknown domain %q", Action{ID: r.ID}.Domain())
		}
		if ids[r.ID] {
			fail(r, "is declared twice, or shares its ID with an action")
		}
		ids[r.ID] = true
		if t := strings.TrimSpace(r.Summary); t == "" || len(t) > 160 || !strings.HasSuffix(t, ".") {
			fail(r, "summary is one sentence of at most 160 characters, ending with a full stop")
		}
		if r.Run == nil {
			fail(r, "has no Run")
		}
		if len(r.On) == 0 {
			fail(r, "follows no event")
		}
		for _, e := range r.On {
			if !events[e] {
				fail(r, "follows %s, which no action emits", e)
			}
		}
	}
	return errs
}
