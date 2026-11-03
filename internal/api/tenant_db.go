package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/tenantdb"
)

// sqlLogger records SQL before it runs. A failed Begin must stop the statement.
type sqlLogger interface {
	BeginSQLAudit(ctx context.Context, e db.SQLAudit) (string, error)
	FinishSQLAudit(ctx context.Context, tenantID, id, status, errText string, rows int) error
	ListSQLAudit(ctx context.Context, tenantID string, limit int) ([]db.SQLAudit, error)
}

// snippetStore keeps saved SQL for one person in one tenant.
type snippetStore interface {
	ListSnippets(ctx context.Context, tenantID, owner string) ([]db.SQLSnippet, error)
	SaveSnippet(ctx context.Context, s db.SQLSnippet) (db.SQLSnippet, error)
	DeleteSnippet(ctx context.Context, tenantID, owner, id string) error
}

func (a *API) databaseRoutes(mux Router) {
	reg := func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, a.flow.Require(h)) }
	reg("GET /v1/tenants/{tenantId}/db", a.dbInfo)
	reg("GET /v1/tenants/{tenantId}/db/tables", a.dbTables)
	reg("POST /v1/tenants/{tenantId}/db/tables", a.dbCreateTable)
	reg("GET /v1/tenants/{tenantId}/db/tables/{table}", a.dbBrowse)
	reg("POST /v1/tenants/{tenantId}/db/tables/{table}/columns", a.dbAddColumn)
	reg("POST /v1/tenants/{tenantId}/db/tables/{table}/rows", a.dbInsertRow)
	reg("PUT /v1/tenants/{tenantId}/db/tables/{table}/rows", a.dbUpdateRow)
	reg("DELETE /v1/tenants/{tenantId}/db/tables/{table}/rows", a.dbDeleteRow)
	reg("POST /v1/tenants/{tenantId}/db/query", a.dbQuery)
	reg("GET /v1/tenants/{tenantId}/db/snippets", a.dbListSnippets)
	reg("POST /v1/tenants/{tenantId}/db/snippets", a.dbSaveSnippet)
	reg("DELETE /v1/tenants/{tenantId}/db/snippets/{snippetId}", a.dbDeleteSnippet)
	reg("GET /v1/tenants/{tenantId}/db/audit", a.dbAudit)
	reg("POST /v1/tenants/{tenantId}/db/ask", a.dbAsk)
	reg("GET /v1/tenants/{tenantId}/db/templates", a.dbTemplates)
	reg("POST /v1/tenants/{tenantId}/db/templates/{templateId}", a.dbApplyTemplate)
	reg("GET /v1/tenants/{tenantId}/db/backups", a.dbBackups)
	reg("POST /v1/tenants/{tenantId}/db/backups", a.dbCreateBackup)
	reg("POST /v1/tenants/{tenantId}/db/backups/{backupId}/restore", a.dbRestoreBackup)
}

func (a *API) logger() sqlLogger {
	if a.sqlLog != nil {
		return a.sqlLog
	}
	if a.db != nil {
		return a.db
	}
	return nil
}

func (a *API) snippetDB() snippetStore {
	if a.snippets != nil {
		return a.snippets
	}
	if a.db != nil {
		return a.db
	}
	return nil
}

func (a *API) backupDB() tenantdb.BackupStore {
	if a.backups != nil {
		return a.backups
	}
	if a.db != nil {
		return db.TenantBackups{Pool: a.db.Pool}
	}
	return &tenantdb.TODOStore{}
}

func (a *API) dbGate() *tenantdb.Gate {
	if a.tenantDB != nil {
		return a.tenantDB
	}
	g := tenantdb.Guard{}
	if a.cfg != nil {
		g.Timeout = a.cfg.TenantDB.Timeout()
		g.MaxRows = a.cfg.TenantDB.Rows()
	}
	return tenantdb.NewGate(nil, g)
}

// dbMember allows organization members only. A platform grant is not enough:
// this surface reads and writes the tenant's own rows.
func (a *API) dbMember(w http.ResponseWriter, r *http.Request) (*caller, *tenantView, bool) {
	c, ok := a.resolve(w, r)
	if !ok {
		return nil, nil, false
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	if !ok {
		return nil, nil, false
	}
	if !t.member() {
		auth.Problem(w, http.StatusForbidden, "Database access is limited to people in this organization.")
		return nil, nil, false
	}
	return c, t, true
}

func (a *API) dbInfo(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	host, port, ssl := "", 5432, "require"
	timeout, rows := 8000, 200
	if a.cfg != nil {
		host = a.cfg.TenantDB.Host
		port = a.cfg.TenantDB.PortOrDefault()
		ssl = a.cfg.TenantDB.SSL()
		timeout = int(a.cfg.TenantDB.Timeout().Milliseconds())
		rows = a.cfg.TenantDB.Rows()
	}
	target, err := tenantdb.TargetOf(t.ID)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, "This organization doesn't have a database name yet.")
		return
	}
	ready := false
	if d := a.dbGate().Dial; d != nil {
		ready = d.Ready(target)
	}
	info, err := tenantdb.Describe(t.ID, host, port, ssl, ready, timeout, rows)
	if err != nil {
		writeDBErr(w, err)
		return
	}
	writeJSON(w, info)
}

func (a *API) dbTemplates(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.dbMember(w, r); !ok {
		return
	}
	writeJSON(w, map[string]any{"templates": tenantdb.Templates()})
}

func (a *API) dbAsk(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.dbMember(w, r)
	if !ok {
		return
	}
	var body struct {
		Prompt string `json:"prompt"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	asker := a.asker
	if asker == nil {
		asker = tenantdb.StubAsker{}
	}
	out, err := asker.Ask(r.Context(), t.ID, c.id.Sub, body.Prompt)
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, out)
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		auth.Problem(w, http.StatusBadRequest, "The request body isn't valid.")
		return false
	}
	return true
}

func writeDBErr(w http.ResponseWriter, err error) {
	var confirm *tenantdb.ConfirmError
	if errors.As(err, &confirm) {
		writeStatus(w, http.StatusConflict, map[string]any{
			"message": confirm.Error(), "confirmation_required": true, "kind": confirm.Kind, "status": http.StatusConflict,
		})
		return
	}
	code := http.StatusBadRequest
	switch {
	case errors.Is(err, tenantdb.ErrReadOnly):
		code = http.StatusForbidden
	case errors.Is(err, tenantdb.ErrNotConfigured), errors.Is(err, tenantdb.ErrAudit):
		code = http.StatusServiceUnavailable
	case errors.Is(err, tenantdb.ErrSuperuser), errors.Is(err, tenantdb.ErrRoleMismatch), errors.Is(err, tenantdb.ErrDatabaseMismatch), errors.Is(err, tenantdb.ErrForbiddenDatabase):
		code = http.StatusForbidden
	}
	auth.Problem(w, code, tenantdb.Plain(err))
}

func dbSchema(r *http.Request) (string, error) {
	s := r.URL.Query().Get("schema")
	if s == "" {
		s = "public"
	}
	if !tenantdb.VisibleSchema(s) {
		return "", errors.New("that schema isn't part of your database")
	}
	return s, nil
}

func dbTable(r *http.Request) (string, error) {
	n := r.PathValue("table")
	if !tenantdb.SafeIdent(n) {
		return "", errors.New("that table isn't available")
	}
	return n, nil
}

func (a *API) auditReject(w http.ResponseWriter, r *http.Request, e db.SQLAudit, err error) {
	e.Status = "rejected"
	e.Error = tenantdb.Plain(err)
	if log := a.logger(); log != nil {
		_, _ = log.BeginSQLAudit(r.Context(), e)
	}
	writeDBErr(w, err)
}

// auditRun records the attempt, runs fn, then records the outcome.
// fn is not called when the audit log cannot be opened.
func (a *API) auditRun(w http.ResponseWriter, r *http.Request, e db.SQLAudit, fn func() (int, error)) bool {
	log := a.logger()
	if log == nil {
		auth.Problem(w, http.StatusServiceUnavailable, tenantdb.Plain(tenantdb.ErrAudit))
		return false
	}
	e.Status = "started"
	id, err := log.BeginSQLAudit(r.Context(), e)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, tenantdb.Plain(tenantdb.ErrAudit))
		return false
	}
	n, err := fn()
	if err != nil {
		_ = log.FinishSQLAudit(r.Context(), e.TenantID, id, "error", tenantdb.Plain(err), 0)
		writeDBErr(w, err)
		return false
	}
	_ = log.FinishSQLAudit(r.Context(), e.TenantID, id, "ok", "", n)
	return true
}

func auditBase(c *caller, t *tenantView, action, stmt, kind string, readOnly, confirm bool) db.SQLAudit {
	return db.SQLAudit{
		TenantID: t.ID, Actor: c.id.Sub, ActorName: c.id.Name, Action: action,
		Statement: stmt, Kind: kind, ReadOnly: readOnly, Confirmed: confirm,
	}
}

type cellMap map[string]any

func (m *cellMap) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		val, err := coerceJSON(v)
		if err != nil {
			return err
		}
		out[k] = val
	}
	*m = out
	return nil
}

func coerceJSON(raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	switch raw[0] {
	case '"':
		var s string
		return s, json.Unmarshal(raw, &s)
	case 't', 'f':
		var b bool
		return b, json.Unmarshal(raw, &b)
	case '{', '[':
		return nil, errors.New("nested value")
	default:
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.UseNumber()
		var n json.Number
		if err := dec.Decode(&n); err != nil {
			return nil, err
		}
		if i, err := n.Int64(); err == nil {
			return i, nil
		}
		return n.Float64()
	}
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
