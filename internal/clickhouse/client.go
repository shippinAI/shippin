// Package clickhouse is the panel's connection to ClickHouse, which holds time series and events:
// host and guest metrics first (CORE-01, OBS-01). It speaks ClickHouse's HTTP interface with the
// standard library, so it adds no dependency. PostgreSQL stays the authority for state.
package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var nameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)

// Client talks to one ClickHouse database.
type Client struct {
	base     *url.URL // scheme and host only
	user     string
	password string
	database string
	http     *http.Client
}

// Open validates a URL of the form http(s)://user:password@host:port/database. It does not connect.
func Open(raw string) (*Client, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("clickhouse: the URL must be http(s)://user:password@host:port/database")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("clickhouse: the URL takes no query or fragment")
	}
	db := strings.Trim(u.Path, "/")
	if !nameRe.MatchString(db) {
		return nil, errors.New("clickhouse: the URL must name a database, such as /shippin")
	}
	c := &Client{base: &url.URL{Scheme: u.Scheme, Host: u.Host}, database: db, http: &http.Client{Timeout: 30 * time.Second}}
	if u.User != nil {
		c.user = u.User.Username()
		c.password, _ = u.User.Password()
	}
	return c, nil
}

// URL is where this client connects, without credentials.
func (c *Client) URL() string { return c.base.String() + "/" + c.database }

// Endpoint names the server and database, for showing to people.
func (c *Client) Endpoint() string { return c.base.Host + "/" + c.database }

// Database is the database every statement runs in.
func (c *Client) Database() string { return c.database }

// Ping succeeds when the server answers and accepts these credentials for the database.
func (c *Client) Ping(ctx context.Context) error {
	var out []struct {
		One uint8 `json:"one"`
	}
	return c.Query(ctx, "SELECT 1 AS one", nil, &out)
}

// Exec runs one statement that returns no rows.
func (c *Client) Exec(ctx context.Context, sql string, params map[string]string) error {
	body, err := c.do(ctx, sql, params, nil)
	if body != nil {
		body.Close()
	}
	return err
}

// Query runs a SELECT with server-side parameters ({name:Type} placeholders bound from params) and
// decodes its rows into out, a pointer to a slice of structs with json tags.
func (c *Client) Query(ctx context.Context, sql string, params map[string]string, out any) error {
	body, err := c.do(ctx, strings.TrimRight(strings.TrimSpace(sql), ";")+" FORMAT JSON", params, nil)
	if err != nil {
		return err
	}
	defer body.Close()
	var res struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(body, 64<<20)).Decode(&res); err != nil {
		return fmt.Errorf("clickhouse: reading the result: %w", err)
	}
	if len(res.Data) == 0 {
		res.Data = []byte("[]")
	}
	return json.Unmarshal(res.Data, out)
}

// Insert writes rows (each JSON-encodable to the table's columns) in one request.
func (c *Client) Insert(ctx context.Context, table string, rows []any) error {
	if !nameRe.MatchString(table) {
		return fmt.Errorf("clickhouse: invalid table name %q", table)
	}
	if len(rows) == 0 {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("clickhouse: encoding a row for %s: %w", table, err)
		}
	}
	body, err := c.do(ctx, "INSERT INTO "+table+" FORMAT JSONEachRow", nil, &buf)
	if body != nil {
		body.Close()
	}
	return err
}

// do sends sql as the query parameter when there is a data body, otherwise as the body.
func (c *Client) do(ctx context.Context, sql string, params map[string]string, data io.Reader) (io.ReadCloser, error) {
	// Numbers come back as JSON numbers on every server version, whatever its default.
	q := url.Values{"database": {c.database}, "output_format_json_quote_64bit_integers": {"0"}}
	for k, v := range params {
		if !nameRe.MatchString(k) {
			return nil, fmt.Errorf("clickhouse: invalid parameter name %q", k)
		}
		q.Set("param_"+k, v)
	}
	var body io.Reader = strings.NewReader(sql)
	if data != nil {
		q.Set("query", sql)
		body = data
	}
	u := *c.base
	u.Path = "/"
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), body)
	if err != nil {
		return nil, err
	}
	if c.user != "" {
		req.Header.Set("X-ClickHouse-User", c.user)
		req.Header.Set("X-ClickHouse-Key", c.password)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: %s is unreachable: %w", c.base.Host, unwrapURL(err))
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("clickhouse: %s answered %d: %s", c.base.Host, res.StatusCode, c.redact(strings.TrimSpace(string(msg))))
	}
	return res.Body, nil
}

func (c *Client) redact(s string) string {
	if c.password != "" {
		s = strings.ReplaceAll(s, c.password, "[redacted]")
	}
	return s
}

func unwrapURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
