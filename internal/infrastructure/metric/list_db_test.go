package metric_test

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/clickhouse"
	"shippin.cloud/console/internal/infrastructure/metric"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/mcpl/mcpltest"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/servers"
)

type history struct{ ch *clickhouse.Client }

func (h history) ClickHouse() *clickhouse.Client { return h.ch }
func (h history) Sampling() ports.Sampling {
	return ports.Sampling{State: "sampling", IntervalSeconds: 30, LastAt: time.Date(2026, 11, 15, 9, 59, 30, 0, time.UTC)}
}

// realClickHouse is the test ClickHouse, migrated; the test is skipped without one.
func realClickHouse(t *testing.T) *clickhouse.Client {
	t.Helper()
	raw := os.Getenv("SHIPPIN_TEST_CLICKHOUSE_URL")
	if raw == "" {
		t.Skip("SHIPPIN_TEST_CLICKHOUSE_URL is not set")
	}
	ch, err := clickhouse.Open(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ch.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return ch
}

const path = "/v1/operator/infrastructure/metrics"

func q(server, guest, metrics, from, to, step string) string {
	v := url.Values{"server_id": {server}, "guest_id": {guest}, "metrics": {metrics}, "from": {from}, "to": {to}, "step": {step}}
	return path + "?" + v.Encode()
}

// Samples with an hour-long hole in the middle come back as buckets with the hole as nulls, the
// average and maximum of each step, and nothing of another guest or metric.
func TestRangeShowsAHoleAsNulls(t *testing.T) {
	ch := realClickHouse(t)
	svcs := mcpl.Services{}
	mcpl.Provide[ports.Metrics](svcs, history{ch})
	h := mcpltest.Serve(t, actions.All, "shippin.infrastructure.metric.", mcpltest.As{Actor: "staff", Services: svcs})
	server := uuid.NewString()
	ctx := context.Background()
	if _, err := h.Pool.Exec(ctx, `INSERT INTO servers(id, tenant_id, slug, label, site, state, created_by, hv_kind)
		VALUES ($1, $2, 'metrics-host', 'Metrics host', 'main', 'planned', 'test', 'incus')`, server, servers.SystemTenantID); err != nil {
		t.Fatal(err)
	}
	// 08:00 to 10:00 in five-minute steps; samples every minute except from 08:30 to 09:30.
	start := time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC)
	var rows []any
	for m := 0; m < 120; m++ {
		if m >= 30 && m < 90 {
			continue
		}
		at := start.Add(time.Duration(m) * time.Minute).Format(metric.TimeFormat)
		rows = append(rows,
			metric.Sample{At: at, ServerID: server, GuestID: "web", Node: "n1", Metric: metric.CPU, Value: float64(m%5) / 10},
			metric.Sample{At: at, ServerID: server, GuestID: "web", Node: "n1", Metric: metric.MemoryUsed, Value: 1000},
			metric.Sample{At: at, ServerID: server, GuestID: "other", Node: "n1", Metric: metric.CPU, Value: 1},
			metric.Sample{At: at, ServerID: server, GuestID: "", Node: "n1", Metric: metric.CPU, Value: 0.9})
	}
	if err := ch.Insert(ctx, metric.Table, rows); err != nil {
		t.Fatal(err)
	}

	got := h.Call("GET", q(server, "web", "cpu,memory_used_bytes,net_in_bytes_per_second", "2026-11-15T08:02:00Z", "2026-11-15T10:00:00Z", "300"), "", 200)
	if got["from"] != "2026-11-15T08:00:00Z" || got["to"] != "2026-11-15T10:00:00Z" || got["step_seconds"] != float64(300) || got["guest_id"] != "web" {
		t.Fatalf("range %v %v %v", got["from"], got["to"], got["step_seconds"])
	}
	times := got["times"].([]any)
	if len(times) != 24 || times[0] != "2026-11-15T08:00:00Z" || times[23] != "2026-11-15T09:55:00Z" {
		t.Fatalf("times %v", times)
	}
	series := got["series"].([]any)
	if len(series) != 3 {
		t.Fatalf("series %v", series)
	}
	cpu, mem, net := series[0].(map[string]any), series[1].(map[string]any), series[2].(map[string]any)
	if cpu["metric"] != "cpu" || cpu["unit"] != "fraction" || mem["unit"] != "bytes" || net["metric"] != "net_in_bytes_per_second" {
		t.Fatalf("series %v %v %v", cpu["metric"], cpu["unit"], mem["unit"])
	}
	buckets := cpu["buckets"].([]any)
	for i, b := range buckets {
		hole := i >= 6 && i < 18 // 08:30 to 09:30
		if hole != (b == nil) {
			t.Fatalf("bucket %d (%s) = %v, hole %v", i, times[i], b, hole)
		}
		if b == nil {
			continue
		}
		m := b.(map[string]any)
		// Each five minutes holds the values 0, 0.1, 0.2, 0.3 and 0.4 of this guest alone.
		if m["samples"] != float64(5) || m["max"] != 0.4 || m["avg"].(float64) < 0.199 || m["avg"].(float64) > 0.201 {
			t.Fatalf("bucket %d = %v", i, m)
		}
	}
	if b := mem["buckets"].([]any); b[0].(map[string]any)["avg"] != float64(1000) || b[10] != nil {
		t.Fatalf("memory %v", b)
	}
	for _, b := range net["buckets"].([]any) {
		if b != nil {
			t.Fatalf("a metric with no samples must be all nulls, never zeros: %v", net["buckets"])
		}
	}
	if s := got["sampler"].(map[string]any); s["state"] != "sampling" || s["last_sampled_at"] != "2026-11-15T09:59:30Z" {
		t.Fatalf("sampler %v", s)
	}

	// The host's own samples are guest_id "".
	host := h.Call("GET", q(server, "", "cpu", "2026-11-15T08:00:00Z", "2026-11-15T08:10:00Z", "600"), "", 200)
	if b := host["series"].([]any)[0].(map[string]any)["buckets"].([]any); len(b) != 1 || b[0].(map[string]any)["max"] != 0.9 || b[0].(map[string]any)["samples"] != float64(10) {
		t.Fatalf("host %v", b)
	}

	// Refusals: each says what is wrong.
	for _, c := range []struct{ url, want string }{
		{q(server, "web", "cpu,swap", "2026-11-15T08:00:00Z", "2026-11-15T10:00:00Z", "300"), `Unknown metric "swap"`},
		{q(server, "web", "", "2026-11-15T08:00:00Z", "2026-11-15T10:00:00Z", "300"), "Name between 1 and"},
		{q(server, "web", "cpu", "2026-11-15T10:00:00Z", "2026-11-15T08:00:00Z", "300"), "to must be after from"},
		{q(server, "web", "cpu", "yesterday", "2026-11-15T08:00:00Z", "300"), "from must be an RFC 3339 time"},
		{q(server, "web", "cpu", "2026-01-01T00:00:00Z", "2026-11-15T08:00:00Z", "86400"), "at most 92 days"},
		{q(server, "web", "cpu", "2026-11-01T00:00:00Z", "2026-11-15T08:00:00Z", "60"), "use a step of at least"},
		{q(server, "web", "cpu", "2026-11-15T08:00:00Z", "2026-11-15T10:00:00Z", "5"), "step must be a whole number of seconds, at least 10"},
		{q("not-a-host", "web", "cpu", "2026-11-15T08:00:00Z", "2026-11-15T10:00:00Z", "300"), "server_id must be"},
	} {
		got := h.Call("GET", c.url, "", 400)
		if msg, _ := got["message"].(string); !strings.Contains(msg, c.want) {
			t.Errorf("%s: %q, want %q", c.url, msg, c.want)
		}
	}
	if got := h.Call("GET", q(uuid.NewString(), "", "cpu", "2026-11-15T08:00:00Z", "2026-11-15T10:00:00Z", "300"), "", 404); !strings.Contains(got["message"].(string), "no host") {
		t.Fatalf("unknown host: %v", got)
	}

	// With ClickHouse turned off the read says so rather than answering empty.
	off := mcpl.Services{}
	mcpl.Provide[ports.Metrics](off, history{})
	h.As.Services = off
	if got := h.Call("GET", q(server, "web", "cpu", "2026-11-15T08:00:00Z", "2026-11-15T10:00:00Z", "300"), "", 503); !strings.Contains(got["message"].(string), "ClickHouse is not connected") {
		t.Fatalf("without ClickHouse: %v", got)
	}
}

// The staff permission is the host read permission of the infrastructure pages; the operator gate in
// internal/api enforces it (TestStaffMatrixDenyByDefault and TestMetricsNeedInfraRead there).
func TestRangeNeedsInfraRead(t *testing.T) {
	for _, g := range actions.All {
		if g.ID == "shippin.infrastructure.metric.list" {
			if g.Perm != "infra.read" || g.Access != mcpl.Staff || g.Route != "GET "+path || g.Risk != mcpl.Read {
				t.Fatalf("%+v", g.Action)
			}
			return
		}
	}
	t.Fatal("the action is not registered")
}

// KeepFor changes the real table's TTL, and reading it back sees the change.
func TestKeepForChangesTheRealTTL(t *testing.T) {
	ch := realClickHouse(t)
	ctx := context.Background()
	t.Cleanup(func() { _, _ = metric.KeepFor(context.Background(), ch, 30) })
	if _, err := metric.KeepFor(ctx, ch, 30); err != nil {
		t.Fatal(err)
	}
	if days, err := metric.KeptFor(ctx, ch); err != nil || days != 30 {
		t.Fatalf("kept for %d, %v", days, err)
	}
	if changed, err := metric.KeepFor(ctx, ch, 30); err != nil || changed {
		t.Fatalf("an unchanged TTL was altered: %v %v", changed, err)
	}
	if changed, err := metric.KeepFor(ctx, ch, 7); err != nil || !changed {
		t.Fatalf("KeepFor 7: %v %v", changed, err)
	}
	if days, err := metric.KeptFor(ctx, ch); err != nil || days != 7 {
		t.Fatalf("kept for %d, %v", days, err)
	}
	if _, err := metric.KeepFor(ctx, ch, 0); err == nil {
		t.Fatal("0 days accepted")
	}
}
