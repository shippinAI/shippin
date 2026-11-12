// Package mcpl is Shippin's MCPL contract: every product action is described once, the same way, so a
// person in the panel, the HTTP API and an AI client acting for that person all run the same handler,
// permission check and audit. See docs/mcpl.md.
//
// A context declares its actions next to its routes. The conformance test in internal/api checks every
// served /v1 route against them and keeps a shrinking list of the routes not yet described.
package mcpl

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Version of Shippin's MCPL contract. Bump it when a rule below changes.
const Version = "1"

// Domains are the MCPL domains, in order: the contexts of docs/architecture.md, plus public (no
// sign-in) and auth (getting a session). One vocabulary for action IDs, events and UI actions.
var Domains = []string{"public", "auth", "access", "identity", "tenancy", "infrastructure", "services", "runtime", "platform", "integrations"}

// Risk says what an action does. It decides how an AI client may run it: reads freely, writes with
// the person's intent, destructive ones only after the person confirms.
type Risk string

const (
	Read        Risk = "read"
	Write       Risk = "write"
	Destructive Risk = "destructive"
)

// Access says who may call an action, before its permission.
type Access string

const (
	Public   Access = "public"    // no session
	SignedIn Access = "signed-in" // any session; the handler decides the rest
	Staff    Access = "staff"     // the control plane; Perm names the permission
	Member   Access = "member"    // a tenant member; the tenant's own roles apply
)

// Action is one thing a person can do, described once.
type Action struct {
	// ID is shippin.<domain>.<entity>.<verb>, for example shippin.tenancy.plan.update.
	ID string `json:"id"`
	// Route is "METHOD /path", exactly as served.
	Route string `json:"route"`
	// Access and Perm: who may call it. Perm is the catalog permission for staff actions.
	Access Access `json:"access"`
	Perm   string `json:"perm,omitempty"`
	Risk   Risk   `json:"risk"`
	// Reason: the input carries a `reason`, kept in the audit log with the change.
	Reason bool `json:"reason"`
	// Summary is one plain sentence a person or a model reads to choose the action.
	Summary string `json:"summary"`
	// Event is emitted after the server has done the action: shippin.<domain>.<entity>.<past verb>.
	Event string `json:"event,omitempty"`
}

// Domain is the action's domain, from its ID.
func (a Action) Domain() string {
	parts := strings.Split(a.ID, ".")
	if len(parts) < 4 {
		return ""
	}
	return parts[1]
}

var (
	idPattern    = regexp.MustCompile(`^shippin\.[a-z]+\.[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*\.[a-z][a-z_]*$`)
	eventPattern = regexp.MustCompile(`^shippin\.[a-z]+\.[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*\.[a-z][a-z_]*$`)
	routePattern = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE) /v1(/[a-z0-9{][a-zA-Z0-9_.{}-]*)+$`)
	readVerbs    = []string{"list", "get", "check", "search", "export"}
	deleteVerbs  = []string{"delete", "remove", "revoke", "reset", "clear"}
)

// Validate checks every action against the rules in docs/mcpl.md and returns each problem found.
func Validate(actions []Action) []error {
	var errs []error
	fail := func(a Action, f string, args ...any) {
		errs = append(errs, fmt.Errorf("%s (%s): %s", a.ID, a.Route, fmt.Sprintf(f, args...)))
	}
	ids, routes := map[string]bool{}, map[string]bool{}
	for _, a := range actions {
		if !idPattern.MatchString(a.ID) {
			fail(a, "ID must be shippin.<domain>.<entity>.<verb> in lower case")
		}
		if !slices.Contains(Domains, a.Domain()) {
			fail(a, "unknown domain %q", a.Domain())
		}
		if ids[a.ID] {
			fail(a, "duplicate ID")
		}
		if routes[a.Route] {
			fail(a, "route described twice")
		}
		ids[a.ID], routes[a.Route] = true, true
		if !routePattern.MatchString(a.Route) {
			fail(a, "route must be METHOD /v1/... with lower-case segments")
		}
		method, _, _ := strings.Cut(a.Route, " ")
		verb := a.ID[strings.LastIndex(a.ID, ".")+1:]
		switch a.Risk {
		case Read:
			if method != "GET" && !slices.Contains(readVerbs, verb) {
				fail(a, "a read is a GET or a %v verb", readVerbs)
			}
		case Write:
			if method == "GET" || method == "DELETE" {
				fail(a, "a write is not a %s", method)
			}
		case Destructive:
			if method == "GET" {
				fail(a, "a destructive action is not a GET")
			}
		default:
			fail(a, "risk must be read, write or destructive")
		}
		if method == "GET" && a.Risk != Read {
			fail(a, "a GET is a read")
		}
		if method == "DELETE" && a.Risk != Destructive {
			fail(a, "a DELETE is destructive")
		}
		if method == "DELETE" && !slices.Contains(deleteVerbs, verb) {
			fail(a, "a DELETE's verb is one of %v", deleteVerbs)
		}
		switch a.Access {
		case Public, SignedIn, Member:
		case Staff:
			if a.Perm == "" {
				fail(a, "a staff action names its permission")
			}
		default:
			fail(a, "access must be public, signed-in, staff or member")
		}
		if a.Risk != Read && a.Access == Public && !slices.Contains([]string{"public", "auth"}, a.Domain()) {
			fail(a, "only public and auth actions change anything without a session")
		}
		if s := strings.TrimSpace(a.Summary); s == "" || len(s) > 160 || !strings.HasSuffix(s, ".") {
			fail(a, "summary is one sentence of at most 160 characters, ending with a full stop")
		}
		if a.Event != "" {
			if !eventPattern.MatchString(a.Event) {
				fail(a, "event must be shippin.<domain>.<entity>.<past verb>")
			} else if d := strings.Split(a.Event, ".")[1]; d != a.Domain() {
				fail(a, "event domain %q differs from the action's %q", d, a.Domain())
			}
			if a.Risk == Read {
				fail(a, "a read emits no event")
			}
		}
	}
	return errs
}

// RiskOf is the default risk of an HTTP method: GET reads, DELETE destroys, everything else writes.
func RiskOf(route string) Risk {
	switch method, _, _ := strings.Cut(route, " "); method {
	case "GET":
		return Read
	case "DELETE":
		return Destructive
	}
	return Write
}
