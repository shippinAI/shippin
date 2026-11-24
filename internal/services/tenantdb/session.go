package tenantdb

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Rows is a JSON-friendly result grid.
type Rows struct {
	Cols []string
	Data [][]any
}

// Conn is one session already authenticated as a database role.
type Conn interface {
	Identity(ctx context.Context) (Identity, error)
	Apply(ctx context.Context, g Guard) error
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (int64, error)
	Close()
}

// Dialer opens a connection as the target role to the target database.
// Implementations must not substitute a superuser or another database.
type Dialer interface {
	Dial(ctx context.Context, t Target) (Conn, error)
	Ready(t Target) bool
}

// Open dials, checks the live role and database, then applies the session guard.
// A second identity check runs after the guard so a startup change cannot stick.
func Open(ctx context.Context, d Dialer, tenantID string, g Guard) (Conn, Target, error) {
	t, err := TargetOf(tenantID)
	if err != nil {
		return nil, Target{}, err
	}
	if d == nil {
		return nil, t, ErrNotConfigured
	}
	c, err := d.Dial(ctx, t)
	if err != nil {
		return nil, t, err
	}
	close := true
	defer func() {
		if close {
			c.Close()
		}
	}()
	if err := checkIdentity(ctx, c, t); err != nil {
		return nil, t, err
	}
	if err := c.Apply(ctx, g.withDefaults()); err != nil {
		return nil, t, err
	}
	if err := checkIdentity(ctx, c, t); err != nil {
		return nil, t, err
	}
	close = false
	return c, t, nil
}

func checkIdentity(ctx context.Context, c Conn, t Target) error {
	id, err := c.Identity(ctx)
	if err != nil {
		return fmt.Errorf("could not verify the database session")
	}
	return Authorize(t, id)
}

// PGDialer connects with pgx as the tenant role. The password comes from
// Secrets, keyed by role name. The browser never receives it.
//
// Hostd keeps these passwords in sops and does not return them today. Until a
// pooler or a hostd secret fetch exists, Ready is false and queries answer
// "not configured" rather than falling back to a superuser.
type PGDialer struct {
	Host    string
	Port    int
	SSLMode string
	Timeout time.Duration
	Secrets map[string]string
}

// DialerFrom is the production dialer. An empty host leaves queries unconfigured
// instead of borrowing the console's own database role.
func DialerFrom(host string, port int, ssl string, timeout time.Duration, secrets map[string]string) Dialer {
	if host == "" {
		return nil
	}
	return &PGDialer{Host: host, Port: port, SSLMode: ssl, Timeout: timeout, Secrets: secrets}
}

func (d *PGDialer) Ready(t Target) bool {
	if d == nil || d.Host == "" || len(d.Secrets) == 0 {
		return false
	}
	if err := t.Check(); err != nil {
		return false
	}
	_, ok := d.Secrets[t.Role]
	return ok && d.Secrets[t.Role] != ""
}

func (d *PGDialer) Dial(ctx context.Context, t Target) (Conn, error) {
	if err := t.Check(); err != nil {
		return nil, err
	}
	pw := ""
	if d != nil {
		pw = d.Secrets[t.Role]
	}
	cfg, err := d.connConfig(t, pw)
	if err != nil {
		return nil, err
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, scrub(err, pw)
	}
	return &pgConn{c: conn, secret: pw}, nil
}

func (d *PGDialer) connConfig(t Target, password string) (*pgx.ConnConfig, error) {
	if d == nil || d.Host == "" || strings.ContainsAny(d.Host, "/@? #") {
		return nil, ErrNotConfigured
	}
	if password == "" {
		return nil, ErrNotConfigured
	}
	if err := t.Check(); err != nil {
		return nil, err
	}
	port := d.Port
	if port == 0 {
		port = 5432
	}
	sslmode := d.SSLMode
	if sslmode == "" {
		sslmode = "require"
	}
	switch sslmode {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		return nil, fmt.Errorf("unsupported sslmode")
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	// Password stays off the URL so driver errors cannot echo it.
	raw := fmt.Sprintf("postgres://%s@%s:%d/%s?sslmode=%s&statement_timeout=%d&search_path=public&connect_timeout=5",
		url.QueryEscape(t.Role), d.Host, port, url.QueryEscape(t.Database), url.QueryEscape(sslmode), timeout.Milliseconds())
	cfg, err := pgx.ParseConfig(raw)
	if err != nil {
		return nil, err
	}
	cfg.Password = password
	cfg.User = t.Role
	cfg.Database = t.Database
	if cfg.RuntimeParams == nil {
		cfg.RuntimeParams = map[string]string{}
	}
	delete(cfg.RuntimeParams, "role")
	delete(cfg.RuntimeParams, "session_authorization")
	cfg.RuntimeParams["search_path"] = "public"
	cfg.RuntimeParams["statement_timeout"] = strconv.FormatInt(timeout.Milliseconds(), 10)
	if sslmode == "require" {
		cfg.TLSConfig = &tls.Config{ServerName: d.Host, MinVersion: tls.VersionTLS12}
	}
	return cfg, nil
}

func scrub(err error, secret string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
	}
	return fmt.Errorf("could not sign in to your database as its own role: %s", msg)
}

type pgConn struct {
	c      *pgx.Conn
	secret string
}

func (p *pgConn) Identity(ctx context.Context) (Identity, error) {
	var id Identity
	err := p.c.QueryRow(ctx, `SELECT session_user, current_user, current_database(),
		COALESCE((SELECT rolsuper FROM pg_roles WHERE rolname = session_user), true),
		COALESCE((SELECT rolsuper FROM pg_roles WHERE rolname = current_user), true)`).
		Scan(&id.SessionUser, &id.User, &id.Database, &id.SessionSuper, &id.Super)
	return id, err
}

func (p *pgConn) Apply(ctx context.Context, g Guard) error {
	g = g.withDefaults()
	ro := "off"
	if g.ReadOnly {
		ro = "on"
	}
	_, err := p.c.Exec(ctx, `SELECT set_config('statement_timeout', $1, false),
		set_config('lock_timeout', '3s', false),
		set_config('idle_in_transaction_session_timeout', '15s', false),
		set_config('search_path', 'public', false),
		set_config('default_transaction_read_only', $2, false)`,
		strconv.FormatInt(g.Timeout.Milliseconds(), 10)+"ms", ro)
	return err
}

func (p *pgConn) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := p.c.Query(ctx, sql, args...)
	if err != nil {
		return Rows{}, scrub(err, p.secret)
	}
	defer rows.Close()
	out := Rows{}
	for _, f := range rows.FieldDescriptions() {
		out.Cols = append(out.Cols, f.Name)
	}
	for rows.Next() {
		if len(out.Data) >= 1001 {
			break
		}
		vals, err := rows.Values()
		if err != nil {
			return Rows{}, err
		}
		for i, v := range vals {
			vals[i] = normValue(v)
		}
		out.Data = append(out.Data, vals)
	}
	return out, rows.Err()
}

func (p *pgConn) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := p.c.Exec(ctx, sql, args...)
	if err != nil {
		return 0, scrub(err, p.secret)
	}
	return tag.RowsAffected(), nil
}

func (p *pgConn) Close() { _ = p.c.Close(context.Background()) }

func normValue(v any) any {
	switch t := v.(type) {
	case nil, string, bool, float32, float64, int, int8, int16, int32, int64, uint, uint16, uint32, uint64:
		return v
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	case []byte:
		if len(t) == 0 {
			return ""
		}
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}
