package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/tenantdb"
)

const (
	dbTenantA = "00000000-0000-4000-8000-000000000001"
	dbTenantB = "00000000-0000-4000-8000-000000000002"
)

func TestDatabaseIsMemberScopedAndRoleBound(t *testing.T) {
	a, dial := dbFixture(t, dbTenantA, true)
	mux := http.NewServeMux()
	a.databaseRoutes(mux)

	// Another tenant's id is not this caller's, even if the body claims a database.
	rec := dbDo(mux, http.MethodPost, "/v1/tenants/"+dbTenantB+"/db/query", `{"sql":"select 1","database":"vaultix"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant: %d %s", rec.Code, rec.Body)
	}
	if len(dial.targets) != 0 {
		t.Fatal("cross-tenant dialed")
	}

	// A grant holder is not an organization member.
	grant, _ := dbFixture(t, dbTenantA, false)
	gmux := http.NewServeMux()
	grant.databaseRoutes(gmux)
	rec = dbDo(gmux, http.MethodGet, "/v1/tenants/"+dbTenantA+"/db", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("grant: %d %s", rec.Code, rec.Body)
	}

	// Unknown fields cannot retarget the session.
	rec = dbDo(mux, http.MethodPost, "/v1/tenants/"+dbTenantA+"/db/query", `{"sql":"select 1","database":"vaultix"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("extra database field: %d %s", rec.Code, rec.Body)
	}
	if len(dial.targets) != 0 {
		t.Fatal("dialed on rejected body")
	}

	rec = dbDo(mux, http.MethodPost, "/v1/tenants/"+dbTenantA+"/db/query", `{"sql":"set role postgres","confirm":true}`)
	if rec.Code != http.StatusBadRequest || len(dial.targets) != 0 {
		t.Fatalf("set role: %d dials=%d %s", rec.Code, len(dial.targets), rec.Body)
	}

	rec = dbDo(mux, http.MethodPost, "/v1/tenants/"+dbTenantA+"/db/query", `{"sql":"delete from todos"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "confirmation_required") || len(dial.targets) != 0 {
		t.Fatalf("confirm: %d dials=%d %s", rec.Code, len(dial.targets), rec.Body)
	}

	rec = dbDo(mux, http.MethodPost, "/v1/tenants/"+dbTenantA+"/db/query", `{"sql":"delete from todos","read_only":true,"confirm":true}`)
	if rec.Code != http.StatusForbidden || len(dial.targets) != 0 {
		t.Fatalf("read-only: %d %s", rec.Code, rec.Body)
	}

	rec = dbDo(mux, http.MethodGet, "/v1/tenants/"+dbTenantA+"/db/tables/todos?schema=vx_customer", "")
	if rec.Code != http.StatusBadRequest || len(dial.targets) != 0 {
		t.Fatalf("vx schema: %d %s", rec.Code, rec.Body)
	}

	rec = dbDo(mux, http.MethodPost, "/v1/tenants/"+dbTenantA+"/db/query", `{"sql":"select 1","read_only":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("select: %d %s", rec.Code, rec.Body)
	}
	if len(dial.targets) != 1 || dial.targets[0].Database != "tdb_00000000000040008000000000000001" || dial.targets[0].Role != "tn_00000000000040008000000000000001" {
		t.Fatalf("target %#v", dial.targets)
	}
	if !dial.readOnly {
		t.Fatal("select did not open read-only")
	}

	// A session that comes back as a superuser is refused.
	dial.ident = func(tenantdb.Target) tenantdb.Identity {
		return tenantdb.Identity{SessionUser: "postgres", User: "postgres", Database: "postgres", Super: true}
	}
	rec = dbDo(mux, http.MethodPost, "/v1/tenants/"+dbTenantA+"/db/query", `{"sql":"select 1"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("superuser: %d %s", rec.Code, rec.Body)
	}

	entries := a.sqlLog.(*memSQL).byTenant(dbTenantA)
	var sawSelect, sawReject bool
	for _, e := range entries {
		if e.TenantID != dbTenantA {
			t.Fatal("audit mixed tenants")
		}
		if strings.Contains(e.Statement, "select 1") && e.Status == "ok" {
			sawSelect = true
		}
		if strings.Contains(strings.ToLower(e.Statement), "set role") && e.Status == "rejected" {
			sawReject = true
		}
	}
	if !sawSelect || !sawReject {
		t.Fatalf("audit missing select/reject: %#v", entries)
	}

	// Audit failure stops the dial.
	before := len(dial.targets)
	a.sqlLog.(*memSQL).fail = true
	rec = dbDo(mux, http.MethodPost, "/v1/tenants/"+dbTenantA+"/db/query", `{"sql":"select 2"}`)
	if rec.Code != http.StatusServiceUnavailable || len(dial.targets) != before {
		t.Fatalf("audit fail: %d dials=%d/%d %s", rec.Code, len(dial.targets), before, rec.Body)
	}
}

func TestDatabaseConnectionAskBackupsAndSnippets(t *testing.T) {
	a, dial := dbFixture(t, dbTenantA, true)
	a.cfg.TenantDB.Host = "203.0.113.20"
	mux := http.NewServeMux()
	a.databaseRoutes(mux)

	rec := dbDo(mux, http.MethodGet, "/v1/tenants/"+dbTenantA+"/db", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "s3cret") || !strings.Contains(rec.Body.String(), "[password]") {
		t.Fatalf("info: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), `"database":"vaultix"`) || strings.Contains(rec.Body.String(), "vx_") {
		t.Fatalf("info leaked vaultix: %s", rec.Body)
	}
	var info tenantdb.ConnInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Database != "tdb_00000000000040008000000000000001" || info.Snippets["go"] == "" {
		t.Fatalf("%#v", info)
	}

	rec = dbDo(mux, http.MethodPost, "/v1/tenants/"+dbTenantA+"/db/ask", `{"prompt":"customers from this week"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sql":null`) || !strings.Contains(rec.Body.String(), "tenant_ai_subscription") || len(dial.targets) != 0 {
		t.Fatalf("ask: %d dials=%d %s", rec.Code, len(dial.targets), rec.Body)
	}

	rec = dbDo(mux, http.MethodGet, "/v1/tenants/"+dbTenantA+"/db/templates", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "todo") || !strings.Contains(rec.Body.String(), "crm") || !strings.Contains(rec.Body.String(), "blog") {
		t.Fatalf("templates: %s", rec.Body)
	}

	rec = dbDo(mux, http.MethodPost, "/v1/tenants/"+dbTenantA+"/db/backups", `{"note":"before import"}`)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "pending_backend") {
		t.Fatalf("backup: %d %s", rec.Code, rec.Body)
	}
	var created struct {
		Backup tenantdb.Backup `json:"backup"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rec = dbDo(mux, http.MethodPost, "/v1/tenants/"+dbTenantA+"/db/backups/"+created.Backup.ID+"/restore", "")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body)
	}
	// Same id from the other tenant is not found (the store is shared; the handler passes the path tenant).
	other, _ := dbFixture(t, dbTenantB, true)
	other.backups = a.backups
	omux := http.NewServeMux()
	other.databaseRoutes(omux)
	rec = dbDo(omux, http.MethodPost, "/v1/tenants/"+dbTenantB+"/db/backups/"+created.Backup.ID+"/restore", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross restore: %d %s", rec.Code, rec.Body)
	}

	rec = dbDo(mux, http.MethodPost, "/v1/tenants/"+dbTenantA+"/db/snippets", `{"name":"Open todos","sql":"select * from todos where done = false"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("snippet: %d %s", rec.Code, rec.Body)
	}
	rec = dbDo(mux, http.MethodGet, "/v1/tenants/"+dbTenantA+"/db/snippets", "")
	if !strings.Contains(rec.Body.String(), "Open todos") {
		t.Fatal(rec.Body)
	}
	// A different person in the same org does not see it.
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) {
		return dbCaller(dbTenantA, true, "user-2"), true
	}
	rec = dbDo(mux, http.MethodGet, "/v1/tenants/"+dbTenantA+"/db/snippets", "")
	if strings.Contains(rec.Body.String(), "Open todos") {
		t.Fatal("snippet leaked to another member")
	}
}

func TestDatabaseDeleteConfirmsThenRunsAsRole(t *testing.T) {
	a, dial := dbFixture(t, dbTenantA, true)
	mux := http.NewServeMux()
	a.databaseRoutes(mux)
	rec := dbDo(mux, http.MethodDelete, "/v1/tenants/"+dbTenantA+"/db/tables/todos/rows", `{"pk":{"id":"1"},"confirm":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if len(dial.targets) != 1 || dial.targets[0].TenantID != dbTenantA || dial.readOnly {
		t.Fatalf("delete dial %#v ro=%v", dial.targets, dial.readOnly)
	}
	joined := strings.Join(dial.execs, "\n")
	if !strings.Contains(joined, `DELETE FROM "public"."todos"`) || strings.Contains(joined, "vaultix") {
		t.Fatalf("exec %s", joined)
	}
}

func dbFixture(t *testing.T, tenant string, member bool) (*API, *recDial) {
	t.Helper()
	dial := &recDial{}
	a := &API{
		cfg:      testCfg(),
		flow:     &auth.Flow{},
		tenantDB: tenantdb.NewGate(dial, tenantdb.Guard{MaxRows: 50}),
		sqlLog:   &memSQL{},
		snippets: &memSnips{},
		backups:  &tenantdb.TODOStore{},
		resolveCaller: func(http.ResponseWriter, *http.Request) (*caller, bool) {
			return dbCaller(tenant, member, "user-1"), true
		},
	}
	return a, dial
}

func dbCaller(tenant string, member bool, sub string) *caller {
	tv := tenantView{Tenant: db.Tenant{ID: tenant, Name: "Ada Co"}, Roles: []string{"org_owner"}}
	if !member {
		tv.Roles = nil
		tv.grants = []db.Grant{{ID: "g1", TenantID: tenant}}
	}
	return &caller{id: &auth.Identity{Sub: sub, Name: "Ada"}, managers: testCfg().Roles.Managers, tenants: []tenantView{tv}}
}

func dbDo(mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

type recDial struct {
	mu       sync.Mutex
	targets  []tenantdb.Target
	execs    []string
	readOnly bool
	ident    func(tenantdb.Target) tenantdb.Identity
}

func (d *recDial) Ready(tenantdb.Target) bool { return true }

func (d *recDial) Dial(_ context.Context, t tenantdb.Target) (tenantdb.Conn, error) {
	d.mu.Lock()
	d.targets = append(d.targets, t)
	d.mu.Unlock()
	id := tenantdb.Identity{SessionUser: t.Role, User: t.Role, Database: t.Database}
	if d.ident != nil {
		id = d.ident(t)
	}
	return &recConn{d: d, id: id}, nil
}

type recConn struct {
	d  *recDial
	id tenantdb.Identity
}

func (c *recConn) Identity(context.Context) (tenantdb.Identity, error) { return c.id, nil }
func (c *recConn) Apply(_ context.Context, g tenantdb.Guard) error {
	c.d.mu.Lock()
	c.d.readOnly = g.ReadOnly
	c.d.mu.Unlock()
	return nil
}
func (c *recConn) Query(_ context.Context, sql string, _ ...any) (tenantdb.Rows, error) {
	if strings.Contains(sql, "information_schema.columns") {
		return tenantdb.Rows{Cols: []string{"column_name", "udt_name", "nullable", "column_default", "pk"}, Data: [][]any{
			{"id", "uuid", false, "gen_random_uuid()", true},
			{"title", "text", false, "", false},
		}}, nil
	}
	if strings.Contains(sql, "count(*)") {
		return tenantdb.Rows{Cols: []string{"count"}, Data: [][]any{{int64(1)}}}, nil
	}
	return tenantdb.Rows{Cols: []string{"id", "title"}, Data: [][]any{{"1", "milk"}}}, nil
}
func (c *recConn) Exec(_ context.Context, sql string, _ ...any) (int64, error) {
	c.d.mu.Lock()
	c.d.execs = append(c.d.execs, sql)
	c.d.mu.Unlock()
	return 1, nil
}
func (c *recConn) Close() {}

type memSQL struct {
	mu   sync.Mutex
	rows []db.SQLAudit
	fail bool
}

func (m *memSQL) BeginSQLAudit(_ context.Context, e db.SQLAudit) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return "", tenantdb.ErrAudit
	}
	e.ID = "a" + strconvItoa(len(m.rows)+1)
	m.rows = append(m.rows, e)
	return e.ID, nil
}

func (m *memSQL) FinishSQLAudit(_ context.Context, tenantID, id, status, errText string, rows int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rows {
		if m.rows[i].ID == id && m.rows[i].TenantID == tenantID {
			m.rows[i].Status = status
			m.rows[i].Error = errText
			if status == "ok" {
				n := rows
				m.rows[i].RowCount = &n
			}
			return nil
		}
	}
	return tenantdb.ErrAudit
}

func (m *memSQL) ListSQLAudit(_ context.Context, tenantID string, _ int) ([]db.SQLAudit, error) {
	return m.byTenant(tenantID), nil
}

func (m *memSQL) byTenant(tenant string) []db.SQLAudit {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []db.SQLAudit
	for _, e := range m.rows {
		if e.TenantID == tenant {
			out = append(out, e)
		}
	}
	return out
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

type memSnips struct {
	mu    sync.Mutex
	items []db.SQLSnippet
}

func (m *memSnips) ListSnippets(_ context.Context, tenantID, owner string) ([]db.SQLSnippet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []db.SQLSnippet{}
	for _, s := range m.items {
		if s.TenantID == tenantID && s.Owner == owner {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *memSnips) SaveSnippet(_ context.Context, s db.SQLSnippet) (db.SQLSnippet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.ID = "s1"
	m.items = append(m.items, s)
	return s, nil
}

func (m *memSnips) DeleteSnippet(_ context.Context, tenantID, owner, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, s := range m.items {
		if s.ID == id && s.TenantID == tenantID && s.Owner == owner {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return nil
		}
	}
	return http.ErrNoCookie
}
