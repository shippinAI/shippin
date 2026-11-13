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
	"strings"

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
	Perm string
	// Collection: a verb other than list/get/create/update/delete acts on the whole collection, not one item.
	Collection bool
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
	DB     DB
	Actor  string
	Reason string
	// Perms answers field-level permission questions for the caller; nil allows nothing extra.
	Perms func(perm string) bool
	// Audit records a change after it succeeded and before it commits; a failure undoes the change.
	Audit func(action, detail string) error
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
	serve(id string, risk Risk, c *Call, w http.ResponseWriter, r *http.Request)
}

func (d Def[In, Out]) Describe() Spec       { return d.Spec }
func (d Def[In, Out]) Input() reflect.Type  { return reflect.TypeFor[In]() }
func (d Def[In, Out]) Output() reflect.Type { return reflect.TypeFor[Out]() }

func (d Def[In, Out]) serve(id string, risk Risk, c *Call, w http.ResponseWriter, r *http.Request) {
	var in In
	if r.Method != http.MethodGet {
		if err := decode(w, r, &in, c, risk != Read && d.Access == Staff); err != nil {
			problem(w, err)
			return
		}
	}
	bind(r, &in)
	c.DB = c.Pool
	var tx pgx.Tx
	if risk != Read && c.Pool != nil {
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
	if risk != Read && c.Audit != nil {
		if err := c.Audit(id, describe(r, c.Reason)); err != nil {
			log.Printf("audit %s: %v", id, err)
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
	if _, none := any(out).(None); none {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	code := http.StatusOK
	if strings.HasSuffix(id, ".create") {
		code = http.StatusCreated
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(out)
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

func describe(r *http.Request, reason string) string {
	var ids []string
	for _, part := range strings.Split(r.Pattern, "/") {
		if strings.HasPrefix(part, "{") {
			name := strings.Trim(part, "{}")
			ids = append(ids, name+"="+r.PathValue(name))
		}
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
	return Registered{Action: Action{ID: id, Route: route, Access: s.Access, Perm: s.Perm, Risk: risk,
		Reason: s.Access == Staff && risk != Read, Summary: s.Summary}, Runner: r}
}

// Serve runs the action for one request.
func (g Registered) Serve(c *Call, w http.ResponseWriter, r *http.Request) {
	g.Runner.serve(g.ID, g.Risk, c, w, r)
}
