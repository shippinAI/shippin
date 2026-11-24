package tenantdb

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	tenantA = "00000000-0000-4000-8000-000000000001"
	tenantB = "00000000-0000-4000-8000-000000000002"
)

func TestNamesMatchHostdFormula(t *testing.T) {
	db, role, err := Names(tenantA)
	if err != nil {
		t.Fatal(err)
	}
	// hostd starterNames strips dashes and prefixes tdb_ / tn_.
	if db != "tdb_00000000000040008000000000000001" || role != "tn_00000000000040008000000000000001" {
		t.Fatalf("names: %s %s", db, role)
	}
	if _, _, err := Names("not-a-uuid"); err == nil {
		t.Fatal("invalid uuid accepted")
	}
	if _, _, err := Names("00000000-0000-4000-8000-00000000000G"); err == nil {
		t.Fatal("non-hex uuid accepted")
	}
}

func TestTargetRejectsVaultixAndOtherTenants(t *testing.T) {
	a, err := TargetOf(tenantA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := TargetOf(tenantB)
	if err != nil {
		t.Fatal(err)
	}
	if a.Database == b.Database || a.Role == b.Role {
		t.Fatal("two tenants share a database")
	}
	if strings.EqualFold(a.Database, "vaultix") || strings.HasPrefix(a.Database, "vx_") {
		t.Fatal("starter database looks like vaultix")
	}
	a.Database = "vaultix"
	if err := a.Check(); err == nil {
		t.Fatal("vaultix database accepted")
	}
	a, _ = TargetOf(tenantA)
	a.Database = b.Database
	if err := a.Check(); err == nil {
		t.Fatal("cross-tenant database accepted")
	}
}

func TestVisibleSchemaHidesVaultix(t *testing.T) {
	for _, n := range []string{"vx_abc", "vx_", "VX_Secret", "vaultix", "pg_catalog", "information_schema", "pg_toast", "pg_temp_1", ""} {
		if VisibleSchema(n) {
			t.Errorf("visible %q", n)
		}
	}
	for _, n := range []string{"public", "app", "blog"} {
		if !VisibleSchema(n) {
			t.Errorf("hidden %q", n)
		}
	}
}

func TestAuthorize(t *testing.T) {
	a, _ := TargetOf(tenantA)
	b, _ := TargetOf(tenantB)
	ok := Identity{SessionUser: a.Role, User: a.Role, Database: a.Database}
	if err := Authorize(a, ok); err != nil {
		t.Fatal(err)
	}
	cases := []Identity{
		{SessionUser: a.Role, User: a.Role, Database: a.Database, Super: true},
		{SessionUser: a.Role, User: a.Role, Database: a.Database, SessionSuper: true},
		{SessionUser: "postgres", User: "postgres", Database: a.Database},
		{SessionUser: b.Role, User: b.Role, Database: b.Database},
		{SessionUser: a.Role, User: a.Role, Database: "vaultix"},
		{SessionUser: a.Role, User: a.Role, Database: b.Database},
		{SessionUser: a.Role, User: "postgres", Database: a.Database},
	}
	for _, id := range cases {
		if err := Authorize(a, id); err == nil {
			t.Errorf("accepted %#v", id)
		}
	}
}

func TestClassifySafety(t *testing.T) {
	a, _ := TargetOf(tenantA)
	b, _ := TargetOf(tenantB)
	ok := []string{
		"select 1",
		"SELECT id, title FROM todos WHERE done = false",
		"select ';drop table todos'",
		"select $$ SET ROLE postgres $$",
		"select 'vaultix is a product name'",
		"insert into posts (title) values ('hello')",
		"update todos set done = true where id = '1'",
		"with n as (select 1 as id) select * from n",
		"explain select 1",
		"create table todos (id uuid primary key)",
		"create table contacts (company_id uuid references companies (id) on delete set null)",
		"alter table todos add column notes text",
		"comment on table todos is 'tasks'",
	}
	for _, sql := range ok {
		if _, err := Classify(sql, a); err != nil {
			t.Errorf("rejected safe %q: %v", sql, err)
		}
	}
	bad := []string{
		"select 1; drop table todos",
		"set role postgres",
		"set search_path = vx_abc",
		"reset role",
		"select set_config('role', 'postgres', false)",
		"select set_config('search_path', 'vx_secret', false)",
		"select dblink('dbname=vaultix')",
		"select dblink('dbname=" + b.Database + "')",
		"select * from vx_abc.secrets",
		`select * from "vx_abc".secrets`,
		"drop database " + a.Database,
		"create role other login",
		"create function f() returns int language sql as $$ select 1 $$",
		"grant select on todos to public",
		"copy todos to program 'cat'",
		"do $$ begin perform 1; end $$",
		"select pg_read_file('/etc/passwd')",
		"select * from " + b.Database,
		"/* comment */ set role " + b.Role,
		"select 1; --\n drop table todos",
	}
	for _, sql := range bad {
		if _, err := Classify(sql, a); err == nil {
			t.Errorf("accepted unsafe %q", sql)
		}
	}
	if _, err := Classify("select 'dbname=vaultix'", a); err == nil {
		t.Fatal("dbname=vaultix literal accepted")
	}
	own := "select '" + a.Database + "'"
	if _, err := Classify(own, a); err != nil {
		t.Fatalf("own database literal rejected: %v", err)
	}
}

func TestDestructiveNeedsConfirmAndReadOnly(t *testing.T) {
	a, _ := TargetOf(tenantA)
	for _, sql := range []string{
		"delete from todos",
		"delete from todos where id = '1'",
		"drop table todos",
		"truncate todos",
		"alter table todos drop column title",
		"with d as (delete from todos returning *) select * from d",
		"explain analyze delete from todos",
	} {
		c, err := Classify(sql, a)
		if err != nil || c.Kind != "destructive" {
			t.Fatalf("%q kind=%s err=%v", sql, c.Kind, err)
		}
		var confirm *ConfirmError
		if err := CheckRun(c, false, false); !errors.As(err, &confirm) {
			t.Fatalf("%q did not require confirm: %v", sql, err)
		}
		if err := CheckRun(c, false, true); err != nil {
			t.Fatalf("%q confirm: %v", sql, err)
		}
		if err := CheckRun(c, true, true); !errors.Is(err, ErrReadOnly) {
			t.Fatalf("%q read-only: %v", sql, err)
		}
	}
	c, err := Classify("update todos set done = true", a)
	if err != nil || c.Kind != "write" {
		t.Fatal(c, err)
	}
	if err := CheckRun(c, true, false); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if err := CheckRun(c, false, false); err != nil {
		t.Fatal(err)
	}
}

func TestTemplatesAreSafeDDL(t *testing.T) {
	a, _ := TargetOf(tenantA)
	if len(Templates()) != 3 {
		t.Fatal("expected todo, crm, blog")
	}
	for _, tpl := range Templates() {
		if len(tpl.Statements) == 0 {
			t.Fatal(tpl.ID)
		}
		for _, stmt := range tpl.Statements {
			c, err := Classify(stmt, a)
			if err != nil || c.Kind != "ddl" {
				t.Fatalf("%s: %v %#v", tpl.ID, err, c)
			}
		}
	}
}

func TestQueryBuildersRejectInjection(t *testing.T) {
	cols := []Column{{Name: "id", Type: "uuid", PrimaryKey: true}, {Name: "title", Type: "text"}}
	if _, _, _, err := BuildSelect(Browse{Schema: "vx_abc", Table: "todos", Columns: cols}); err == nil {
		t.Fatal("vx schema selectable")
	}
	if _, _, _, err := BuildSelect(Browse{Schema: "public", Table: "todos;drop", Columns: cols}); err == nil {
		t.Fatal("bad table")
	}
	if _, _, _, err := BuildSelect(Browse{Schema: "public", Table: "todos", Columns: cols, Sort: "title;drop", Dir: "asc"}); err == nil {
		t.Fatal("bad sort")
	}
	page, count, args, err := BuildSelect(Browse{
		Schema: "public", Table: "todos", Columns: cols,
		Filters: []Filter{{Column: "title", Op: "eq", Value: "a' OR '1'='1"}},
		Sort:    "title", Dir: "desc", Limit: 10, Offset: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page, "a'") || strings.Contains(count, "a'") {
		t.Fatalf("value interpolated: %s", page)
	}
	if len(args) != 3 || args[0] != "a' OR '1'='1" {
		t.Fatalf("args %#v", args)
	}
	if _, err := BuildCreateTable("vx_notes", []NewColumn{{Name: "id", Type: "text"}}); err == nil {
		t.Fatal("vx table")
	}
	if _, err := BuildCreateTable("todos", []NewColumn{{Name: "id", Type: "text;drop table todos"}}); err == nil {
		t.Fatal("raw type")
	}
	if _, err := BuildCreateTable("todos", []NewColumn{{Name: "id", Type: "uuid", Default: "gen_random_uuid()); drop table todos;--"}}); err == nil {
		t.Fatal("raw default")
	}
	stmt, err := BuildCreateTable("todos", []NewColumn{{Name: "id", Type: "uuid", PrimaryKey: true, Default: "gen_random_uuid()"}})
	if err != nil || !strings.Contains(stmt, "gen_random_uuid()") || strings.Contains(stmt, ";") {
		t.Fatal(stmt, err)
	}
}

func TestOpenRejectsWrongSession(t *testing.T) {
	a, _ := TargetOf(tenantA)
	b, _ := TargetOf(tenantB)
	ctx := context.Background()
	d := &fakeDialer{ident: func(t Target) Identity {
		return Identity{SessionUser: t.Role, User: t.Role, Database: "vaultix"}
	}}
	if _, _, err := Open(ctx, d, tenantA, Guard{}); err == nil {
		t.Fatal("vaultix session accepted")
	}
	d.ident = func(t Target) Identity {
		return Identity{SessionUser: b.Role, User: b.Role, Database: b.Database}
	}
	if _, _, err := Open(ctx, d, tenantA, Guard{}); err == nil {
		t.Fatal("other tenant session accepted")
	}
	d.ident = func(t Target) Identity {
		return Identity{SessionUser: t.Role, User: t.Role, Database: t.Database, Super: true}
	}
	if _, _, err := Open(ctx, d, tenantA, Guard{}); err == nil {
		t.Fatal("superuser accepted")
	}
	d.ident = nil
	c, got, err := Open(ctx, d, tenantA, Guard{ReadOnly: true, Timeout: time.Second, MaxRows: 10})
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if got.Database != a.Database || d.opened[len(d.opened)-1].Database != a.Database {
		t.Fatalf("opened %#v", d.opened)
	}
	if !d.applied.ReadOnly {
		t.Fatal("read-only guard not applied")
	}
}

func TestQueryDoesNotDialWhenRejected(t *testing.T) {
	d := &fakeDialer{}
	g := NewGate(d, Guard{MaxRows: 5, Timeout: time.Second})
	if _, err := g.Query(context.Background(), tenantA, "drop table todos", false, false); !errors.As(err, new(*ConfirmError)) {
		t.Fatal(err)
	}
	if len(d.opened) != 0 {
		t.Fatal("dialed before confirm")
	}
	if _, err := g.Query(context.Background(), tenantA, "set role postgres", false, true); err == nil || len(d.opened) != 0 {
		t.Fatalf("set role dialed: %v %#v", err, d.opened)
	}
	if _, err := g.Query(context.Background(), tenantB, "delete from todos", true, true); !errors.Is(err, ErrReadOnly) || len(d.opened) != 0 {
		t.Fatal(err, d.opened)
	}
	res, err := g.Query(context.Background(), tenantA, "select 1", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.opened[0].Role != "tn_00000000000040008000000000000001" || res.Kind != "read" || !d.applied.ReadOnly {
		t.Fatalf("res=%#v opened=%#v ro=%v", res, d.opened, d.applied.ReadOnly)
	}
}

func TestConnConfigStaysOnTenantRole(t *testing.T) {
	a, _ := TargetOf(tenantA)
	d := &PGDialer{Host: "203.0.113.20", Port: 5432, SSLMode: "require", Secrets: map[string]string{a.Role: "s3cret-value"}}
	if !d.Ready(a) {
		t.Fatal("expected ready")
	}
	b, _ := TargetOf(tenantB)
	if d.Ready(b) {
		t.Fatal("other tenant ready without a secret")
	}
	cfg, err := d.connConfig(a, "s3cret-value")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.User != a.Role || cfg.Database != a.Database {
		t.Fatalf("user=%s db=%s", cfg.User, cfg.Database)
	}
	if strings.Contains(cfg.ConnString(), "s3cret-value") {
		t.Fatal("password leaked into conn string")
	}
	if cfg.RuntimeParams["role"] != "" || cfg.RuntimeParams["search_path"] != "public" {
		t.Fatalf("params %#v", cfg.RuntimeParams)
	}
	if _, err := d.connConfig(a, ""); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	vault := a
	vault.Database = "vaultix"
	if _, err := d.connConfig(vault, "x"); err == nil {
		t.Fatal("vaultix config")
	}
}

func TestDescribeHasNoPassword(t *testing.T) {
	info, err := Describe(tenantA, "db.internal", 5432, "require", false, 8000, 200)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(info.URI, "s3cret") || !strings.Contains(info.URI, "[password]") {
		t.Fatal(info.URI)
	}
	if strings.Contains(info.URI, "vaultix") || info.Database == "vaultix" {
		t.Fatal(info)
	}
	for _, lang := range []string{"javascript", "python", "go"} {
		if !strings.Contains(info.Snippets[lang], "[password]") || strings.Contains(strings.ToLower(info.Snippets[lang]), "vaultix") {
			t.Fatalf("%s snippet: %s", lang, info.Snippets[lang])
		}
	}
}

func TestBackupTODOIsTenantScoped(t *testing.T) {
	s := &TODOStore{}
	b, err := s.Create(context.Background(), tenantA, "ada", "nightly")
	if err != nil || b.Status != "pending_backend" {
		t.Fatal(b, err)
	}
	if err := s.Restore(context.Background(), tenantB, b.ID); !errors.Is(err, ErrBackupNotFound) {
		t.Fatal(err)
	}
	if err := s.Restore(context.Background(), tenantA, b.ID); !errors.Is(err, ErrBackupPending) {
		t.Fatal(err)
	}
	list, err := s.List(context.Background(), tenantB)
	if err != nil || len(list) != 0 {
		t.Fatal(list, err)
	}
}

func TestTablesHideVXEvenIfQueryReturnsThem(t *testing.T) {
	d := &fakeDialer{query: func(sql string, _ []any) (Rows, error) {
		if strings.Contains(sql, "pg_class") {
			if !strings.Contains(sql, `vx\_`) || !strings.Contains(sql, "vaultix") {
				t.Fatalf("list sql does not hide vaultix: %s", sql)
			}
			return Rows{Cols: []string{"s", "n", "e", "c"}, Data: [][]any{
				{"public", "todos", int64(2), ""},
				{"vx_sid", "secrets", int64(9), ""},
				{"vaultix", "items", int64(1), ""},
				{"public", "bad name", int64(1), ""},
			}}, nil
		}
		return Rows{Cols: []string{"count"}, Data: [][]any{{int64(2)}}}, nil
	}}
	tables, err := NewGate(d, Guard{}).Tables(context.Background(), tenantA)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || tables[0].Name != "todos" || tables[0].RowCount == nil || *tables[0].RowCount != 2 {
		t.Fatalf("%#v", tables)
	}
}

type fakeDialer struct {
	opened  []Target
	applied Guard
	ident   func(Target) Identity
	query   func(string, []any) (Rows, error)
	execs   []string
}

func (d *fakeDialer) Ready(Target) bool { return true }

func (d *fakeDialer) Dial(_ context.Context, t Target) (Conn, error) {
	d.opened = append(d.opened, t)
	id := Identity{SessionUser: t.Role, User: t.Role, Database: t.Database}
	if d.ident != nil {
		id = d.ident(t)
	}
	return &fakeConn{d: d, id: id}, nil
}

type fakeConn struct {
	d  *fakeDialer
	id Identity
}

func (c *fakeConn) Identity(context.Context) (Identity, error) { return c.id, nil }
func (c *fakeConn) Apply(_ context.Context, g Guard) error {
	c.d.applied = g
	return nil
}
func (c *fakeConn) Query(_ context.Context, sql string, args ...any) (Rows, error) {
	if c.d.query != nil {
		return c.d.query(sql, args)
	}
	return Rows{Cols: []string{"?column?"}, Data: [][]any{{1}}}, nil
}
func (c *fakeConn) Exec(_ context.Context, sql string, _ ...any) (int64, error) {
	c.d.execs = append(c.d.execs, sql)
	return 1, nil
}
func (c *fakeConn) Close() {}
