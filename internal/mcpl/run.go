package mcpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// An action is a file: internal/<domain>/<entity...>/<verb>.go declares
//
//	var <Verb> = mcpl.Def[In, Out]{Spec: mcpl.Spec{...}, Run: run}
//
// cmd/mcplgen finds it, derives its ID and route from where it is, and writes the registry and the
// panel's action map. Nothing about an action's address is written by hand. Def does the mechanical
// part for every action alike: strict input, path binding, the reason, the audit entry and the errors.

// Spec is what an action file says about itself. Everything else is derived from its location.
type Spec struct {
	// Summary is one plain sentence for a person, or a model, choosing what to do.
	Summary string
	Access  Access
	// Perm is the catalog permission a staff action needs.
	// For a member action Perm is the organization role it needs: "" any member, "manage" a manager,
	// "decide" an owner or admin who approves access. Signed-in and public actions have none.
	Perm string
	// Collection: a verb other than list/get/create/update/delete acts on the whole collection, not one item.
	Collection bool
	// Singleton: there is one of this entity in its parent (an organization's coverage, a person's
	// profile), so its path is singular and carries no item ID.
	Singleton bool
}

// DB is what an action queries: the pool for a read, the change's transaction for anything else.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Call is one run of an action, for the person it runs as.
type Call struct {
	Ctx  context.Context
	Pool *pgxpool.Pool
	// DB is set by the runner: a change runs in one transaction that commits only with its audit entry.
	DB DB
	// Actor is the person the action runs as; "" for a public action.
	Actor string
	// Tenant is the organization a member action runs in, from the route's {tenantId}.
	Tenant string
	Reason string
	// Perms answers field-level permission questions for the caller; nil allows nothing extra.
	Perms func(perm string) bool
	// Audit records a change after it succeeded and before it commits, on the change's own transaction,
	// so the entry and the change are kept or lost together.
	Audit func(q DB, action, detail string) error
	// Publish hands the action's event to subscribers once the change is committed.
	Publish func(Event)
	// Services are what the action uses besides the database; see Need.
	Services Services
}

// Services are the dependencies actions use besides the database, keyed by the interface type the
// action's own package declares. The host provides the implementations, so an action never reaches
// for a global and a test hands it a fake.
type Services map[reflect.Type]any

// Provide makes v available to actions that Need a T.
func Provide[T any](s Services, v T) { s[reflect.TypeFor[T]()] = v }

// Need is the T this install provides, or a plain 503 when it has none (mail not set up, no IdP).
func Need[T any](c *Call) (T, error) {
	v, ok := c.Services[reflect.TypeFor[T]()].(T)
	if !ok {
		var zero T
		return zero, &Error{http.StatusServiceUnavailable, "This install isn't set up for that yet."}
	}
	return v, nil
}

// Can reports whether the caller holds perm, for fields an action shows or takes only with it.
func (c *Call) Can(perm string) bool { return c.Perms != nil && c.Perms(perm) }

// None is the output of an action that returns nothing (204).
type None struct{}

// Items is the output of a list.
type Items[T any] struct {
	Items []T `json:"items"`
}

// Def is an action: its spec and the function that does it.
type Def[In, Out any] struct {
	Spec
	Run func(c *Call, in In) (Out, error)
}

// Runner is a Def with its types erased, as the registry holds it.
type Runner interface {
	Describe() Spec
	Input() reflect.Type
	Output() reflect.Type
	serve(g Registered, c *Call, w http.ResponseWriter, r *http.Request)
}

func (d Def[In, Out]) Describe() Spec       { return d.Spec }
func (d Def[In, Out]) Input() reflect.Type  { return reflect.TypeFor[In]() }
func (d Def[In, Out]) Output() reflect.Type { return reflect.TypeFor[Out]() }

func (d Def[In, Out]) serve(g Registered, c *Call, w http.ResponseWriter, r *http.Request) {
	var in In
	if r.Method != http.MethodGet {
		if err := decode(w, r, &in, c, g.Reason); err != nil {
			problem(w, err)
			return
		}
	}
	bind(r, &in)
	c.DB = c.Pool
	var tx pgx.Tx
	if g.Risk != Read && c.Pool != nil {
		var err error
		if tx, err = c.Pool.Begin(c.Ctx); err != nil {
			problem(w, err)
			return
		}
		defer tx.Rollback(c.Ctx)
		c.DB = tx
	}
	out, err := d.Run(c, in)
	if err != nil {
		problem(w, err)
		return
	}
	subject := subjectOf(r, out)
	if g.Risk != Read && c.Audit != nil {
		if err := c.Audit(c.DB, g.ID, describe(subject, c.Reason)); err != nil {
			log.Printf("audit %s: %v", g.ID, err)
			problem(w, &Error{http.StatusServiceUnavailable, "The change couldn't be audited, so it wasn't made."})
			return
		}
	}
	if tx != nil {
		if err := tx.Commit(c.Ctx); err != nil {
			problem(w, err)
			return
		}
	}
	if g.Event != "" && c.Publish != nil {
		c.Publish(Event{ID: g.Event, Action: g.ID, Actor: c.Actor, Tenant: c.Tenant, Access: g.Access, Perm: g.Perm, Subject: subject, At: time.Now().UTC()})
	}
	if _, none := any(out).(None); none {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	code := http.StatusOK
	if strings.HasSuffix(g.ID, ".create") {
		code = http.StatusCreated
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(out)
}

// subjectOf is what the action acted on: the route's own values (tenantId, coverageTierId), and the
// id of what it created.
func subjectOf(r *http.Request, out any) map[string]string {
	s := map[string]string{}
	for _, part := range strings.Split(r.Pattern, "/") {
		if strings.HasPrefix(part, "{") {
			name := strings.Trim(part, "{}")
			s[name] = r.PathValue(name)
		}
	}
	if r.Method == http.MethodPost {
		var created struct {
			ID any `json:"id"`
		}
		if raw, err := json.Marshal(out); err == nil && json.Unmarshal(raw, &created) == nil && created.ID != nil {
			s["id"] = fmt.Sprint(created.ID)
		}
	}
	return s
}

// decode reads a JSON object, takes its reason out, and decodes the rest strictly into in.
func decode(w http.ResponseWriter, r *http.Request, in any, c *Call, needReason bool) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return Invalid("The request body is too large or unreadable.")
	}
	body := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(raw)) > 0 && json.Unmarshal(raw, &body) != nil {
		return Invalid("The request body must be a JSON object.")
	}
	if v, ok := body["reason"]; ok {
		if json.Unmarshal(v, &c.Reason) != nil {
			return Invalid("reason must be text.")
		}
		delete(body, "reason")
	}
	c.Reason = strings.TrimSpace(c.Reason)
	if needReason && len(c.Reason) < 5 {
		return Invalid("Give a reason of at least 5 characters; the audit log keeps it with the change.")
	}
	rest, _ := json.Marshal(body)
	dec := json.NewDecoder(bytes.NewReader(rest))
	dec.DisallowUnknownFields()
	if err := dec.Decode(in); err != nil {
		return Invalid("The request was not accepted: " + strings.TrimPrefix(err.Error(), "json: ") + ".")
	}
	return nil
}

// bind fills fields tagged path:"name" from the route and query:"name" from the query string,
// including those of embedded structs.
func bind(r *http.Request, in any) { bindValue(r, reflect.ValueOf(in).Elem()) }

func bindValue(r *http.Request, v reflect.Value) {
	if v.Kind() != reflect.Struct {
		return
	}
	for i := range v.NumField() {
		f := v.Type().Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			bindValue(r, v.Field(i))
			continue
		}
		if f.Type.Kind() != reflect.String {
			continue
		}
		if name := f.Tag.Get("path"); name != "" {
			v.Field(i).SetString(r.PathValue(name))
		} else if name := f.Tag.Get("query"); name != "" {
			v.Field(i).SetString(r.URL.Query().Get(name))
		}
	}
}

func describe(subject map[string]string, reason string) string {
	keys := make([]string, 0, len(subject))
	for k := range subject {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	ids := make([]string, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, k+"="+subject[k])
	}
	return strings.TrimSpace(strings.Join(ids, " ") + " · " + reason)
}

// Error is an expected failure with the status and the plain message a person acts on.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

func Invalid(m string) error   { return &Error{http.StatusBadRequest, m} }
func Forbidden(m string) error { return &Error{http.StatusForbidden, m} }
func NotFound(m string) error  { return &Error{http.StatusNotFound, m} }
func Conflict(m string) error  { return &Error{http.StatusConflict, m} }

// problem writes every error the same way: application/problem+json with a plain message.
func problem(w http.ResponseWriter, err error) {
	var e *Error
	var pg *pgconn.PgError
	switch {
	case errors.As(err, &e):
	case errors.Is(err, pgx.ErrNoRows):
		e = &Error{http.StatusNotFound, "Not found."}
	case errors.As(err, &pg) && pg.Code == "23505":
		e = &Error{http.StatusConflict, "That already exists."}
	case errors.As(err, &pg) && pg.Code == "23503":
		e = &Error{http.StatusConflict, "Something else still refers to it."}
	case errors.As(err, &pg) && pg.Code == "23514":
		e = &Error{http.StatusBadRequest, "A value is outside what is allowed."}
	default:
		log.Printf("mcpl action: %v", err)
		e = &Error{http.StatusServiceUnavailable, "That couldn't be done right now. Try again."}
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "about:blank", "title": http.StatusText(e.Status), "status": e.Status, "message": e.Message})
}

// Registered is an action placed in the registry: its description, derived from its location, and its runner.
type Registered struct {
	Action
	Runner Runner `json:"-"`
}

// Register derives an action's route, risk and reason rule from its ID and spec.
func Register(id string, r Runner) Registered {
	s := r.Describe()
	route, err := RouteFor(id, s)
	if err != nil {
		panic(fmt.Sprintf("mcpl: %s: %v", id, err))
	}
	risk := RiskOf(route)
	a := Action{ID: id, Route: route, Access: s.Access, Perm: s.Perm, Risk: risk,
		Reason: s.Access == Staff && risk != Read, Summary: s.Summary}
	if risk != Read {
		a.Event = EventFor(id)
	}
	return Registered{Action: a, Runner: r}
}

// Serve runs the action for one request.
func (g Registered) Serve(c *Call, w http.ResponseWriter, r *http.Request) {
	g.Runner.serve(g, c, w, r)
}
