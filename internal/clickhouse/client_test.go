package clickhouse

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOpenRefusesWhatIsNotADatabaseURL(t *testing.T) {
	for _, raw := range []string{
		"", "clickhouse.local:8123", "ftp://u:p@h:8123/db", "http://u:p@h:8123", "http://u:p@h:8123/",
		"http://u:p@h:8123/db?x=1", "http://u:p@h:8123/db;drop", "http://u:p@h:8123/a/b",
	} {
		if _, err := Open(raw); err == nil {
			t.Errorf("Open(%q) accepted", raw)
		}
	}
	c, err := Open("https://shippin:secret@clickhouse:8443/shippin")
	if err != nil || c.Database() != "shippin" || c.user != "shippin" || c.password != "secret" || c.base.String() != "https://clickhouse:8443" {
		t.Fatalf("Open = %+v, %v", c, err)
	}
}

func TestRequestsCarryCredentialsInHeadersAndBindParameters(t *testing.T) {
	var got *http.Request
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		io.WriteString(w, `{"data":[{"n":"7"}]}`)
	}))
	defer srv.Close()
	c, _ := Open(strings.Replace(srv.URL, "http://", "http://shippin:s3cret@", 1) + "/shippin")
	var out []struct {
		N string `json:"n"`
	}
	if err := c.Query(context.Background(), "SELECT count() AS n FROM t WHERE id = {id:String};", map[string]string{"id": "x'; DROP"}, &out); err != nil {
		t.Fatal(err)
	}
	if got.Header.Get("X-ClickHouse-User") != "shippin" || got.Header.Get("X-ClickHouse-Key") != "s3cret" || got.URL.User != nil {
		t.Fatalf("credentials: headers %v, url user %v", got.Header, got.URL.User)
	}
	q := got.URL.Query()
	if q.Get("database") != "shippin" || q.Get("param_id") != "x'; DROP" || strings.Contains(body, "DROP") {
		t.Fatalf("parameter must travel separately from the SQL: query %v, body %q", q, body)
	}
	if body != "SELECT count() AS n FROM t WHERE id = {id:String} FORMAT JSON" || len(out) != 1 || out[0].N != "7" {
		t.Fatalf("body %q, out %+v", body, out)
	}
	if err := c.Query(context.Background(), "SELECT 1", map[string]string{"bad-name": "x"}, &out); err == nil {
		t.Fatal("an invalid parameter name was accepted")
	}
}

func TestErrorsNameTheServerAndHideThePassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, "Code: 516. Authentication failed: password s3cret is incorrect")
	}))
	defer srv.Close()
	c, _ := Open(strings.Replace(srv.URL, "http://", "http://shippin:s3cret@", 1) + "/shippin")
	err := c.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "answered 403") || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("Ping error = %v", err)
	}
	srv.Close()
	if err := c.Ping(context.Background()); err == nil || !strings.Contains(err.Error(), "unreachable") || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("unreachable error = %v", err)
	}
}

// TestRealServer runs against a live ClickHouse when SHIPPIN_TEST_CLICKHOUSE_URL is set (CI sets it).
func TestRealServer(t *testing.T) {
	raw := os.Getenv("SHIPPIN_TEST_CLICKHOUSE_URL")
	if raw == "" {
		t.Skip("SHIPPIN_TEST_CLICKHOUSE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c, err := Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := c.Migrate(ctx)
	if err != nil || len(again) != 0 {
		t.Fatalf("a second Migrate applied %v, %v", again, err)
	}
	server := uuid.NewString()
	at := time.Now().UTC().Truncate(time.Millisecond)
	rows := []any{
		map[string]any{"at": at.Format("2006-01-02 15:04:05.000"), "server_id": server, "guest_id": "", "node": "n1", "metric": "cpu_fraction", "value": 0.25},
		map[string]any{"at": at.Format("2006-01-02 15:04:05.000"), "server_id": server, "guest_id": "101", "node": "n1", "metric": "memory_used_bytes", "value": 1.5e9},
	}
	if err := c.Insert(ctx, "shippin_metric_samples", rows); err != nil {
		t.Fatal(err)
	}
	var out []struct {
		Guest string  `json:"guest_id"`
		Value float64 `json:"value"`
	}
	if err := c.Query(ctx, "SELECT guest_id, value FROM shippin_metric_samples WHERE server_id = {s:UUID} ORDER BY guest_id", map[string]string{"s": server}, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Guest != "" || out[0].Value != 0.25 || out[1].Value != 1.5e9 {
		t.Fatalf("read back %+v", out)
	}
	if err := c.Query(ctx, "SELECT nope FROM shippin_metric_samples", nil, &out); err == nil {
		t.Fatal("a bad query reported success")
	}
}
