package metric

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"shippin.cloud/console/internal/clickhouse"
	"shippin.cloud/console/internal/hv"
)

// fakeCH is enough of ClickHouse's HTTP interface for the sampler: it answers the TTL lookup, records
// ALTERs and keeps inserted rows.
type fakeCH struct {
	mu     sync.Mutex
	ttl    int
	alters []string
	rows   []Sample
	fail   bool
}

func (f *fakeCH) serve(t *testing.T) *clickhouse.Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		q := r.URL.Query().Get("query")
		switch {
		case strings.HasPrefix(q, "INSERT INTO "+Table):
			if f.fail {
				http.Error(w, "Code: 241. Memory limit exceeded", http.StatusInternalServerError)
				return
			}
			sc := bufio.NewScanner(strings.NewReader(string(body)))
			for sc.Scan() {
				var s Sample
				if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
					t.Errorf("row %q: %v", sc.Text(), err)
				}
				f.rows = append(f.rows, s)
			}
		case strings.Contains(string(body), "system.tables"):
			io.WriteString(w, `{"data":[{"create_table_query":"CREATE TABLE d.shippin_metric_samples (...) TTL toDateTime(at) + toIntervalDay(`+itoa(f.ttl)+`)"}]}`)
		case strings.HasPrefix(string(body), "ALTER TABLE"):
			f.alters = append(f.alters, string(body))
		default:
			t.Errorf("unexpected statement %q %q", q, body)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := clickhouse.Open(srv.URL + "/shippin")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *fakeCH) take() []Sample {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.rows
	f.rows = nil
	return out
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

type fakeLead struct {
	mu            sync.Mutex
	free, holding bool
	released      int
}

func (l *fakeLead) Hold(context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.free {
		l.holding = true
	}
	return l.holding, nil
}

func (l *fakeLead) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holding {
		l.released++
	}
	l.holding = false
}

// meter is a driver that reports what the test sets. hv.Driver is embedded only to satisfy the
// interface; the sampler calls nothing but the meters.
type meter struct {
	hv.Driver
	mu     sync.Mutex
	host   hv.HostMetrics
	guests []hv.GuestReading
	block  bool
}

func (m *meter) HostMetrics(ctx context.Context) (hv.HostMetrics, error) {
	if m.block {
		<-ctx.Done()
		return hv.HostMetrics{}, ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.host, nil
}

func (m *meter) GuestReadings(ctx context.Context) ([]hv.GuestReading, error) {
	if m.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]hv.GuestReading(nil), m.guests...), nil
}

type rig struct {
	s     *Sampler
	ch    *fakeCH
	lead  *fakeLead
	now   time.Time
	cfg   Config
	store *clickhouse.Client
}

func newRig(t *testing.T, targets ...Target) *rig {
	r := &rig{ch: &fakeCH{ttl: 30}, lead: &fakeLead{free: true}, now: time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC), cfg: Defaults}
	r.store = r.ch.serve(t)
	r.s = &Sampler{
		Targets:    func(context.Context) ([]Target, error) { return targets, nil },
		ClickHouse: func() *clickhouse.Client { return r.store },
		Config:     func(context.Context) Config { return r.cfg },
		Lead:       r.lead,
		Now:        func() time.Time { return r.now },
	}
	return r
}

func target(id string, d hv.Driver) Target {
	return Target{ServerID: id, Open: func() (hv.Driver, error) { return d, nil }}
}

// values maps "guest/metric" to the value written, for one tick's rows.
func values(rows []Sample) map[string]float64 {
	out := map[string]float64{}
	for _, r := range rows {
		out[r.GuestID+"/"+r.Metric] = r.Value
	}
	return out
}

const host1 = "11111111-1111-4111-8111-111111111111"

func TestSamplerWritesHostsAndGuestsAndMakesRatesFromCounters(t *testing.T) {
	m := &meter{
		host: hv.HostMetrics{Node: "n1", CPUs: 8, CPU: hv.Ptr(0.25), Load1: hv.Ptr(1.5), MemoryUsed: hv.Ptr[int64](4), MemoryTotal: hv.Ptr[int64](16),
			NetInRate: hv.Ptr(100.0)},
		guests: []hv.GuestReading{
			// Incus-like: CPU time and counters.
			{ID: "web", Status: "Running", CPUSeconds: hv.Ptr(100.0), CPULimit: 2, MemoryUsed: hv.Ptr[int64](10), NetIn: hv.Ptr[int64](1000), NetOut: hv.Ptr[int64](0)},
			// Proxmox-like: a CPU fraction, on another cluster node.
			{ID: "101", Node: "n2", Status: "running", CPU: hv.Ptr(0.5), NetIn: hv.Ptr[int64](50)},
			// Uses all of the host's CPUs.
			{ID: "all", Status: "Running", CPUSeconds: hv.Ptr(10.0)},
			{ID: "off", Status: "stopped", MemoryUsed: hv.Ptr[int64](1)},
		},
	}
	r := newRig(t, target(host1, m))
	r.s.Tick(context.Background())
	first := r.ch.take()
	v := values(first)
	// The first reading of a counter gives no rate: a gap, not a zero.
	if v["/cpu"] != 0.25 || v["/load1"] != 1.5 || v["/memory_total_bytes"] != 16 || v["/net_in_bytes_per_second"] != 100 || v["101/cpu"] != 0.5 || v["web/memory_used_bytes"] != 10 {
		t.Fatalf("first tick: %v", v)
	}
	for _, k := range []string{"web/cpu", "web/net_in_bytes_per_second", "101/net_in_bytes_per_second", "all/cpu", "/net_out_bytes_per_second", "/disk_used_bytes"} {
		if _, ok := v[k]; ok {
			t.Errorf("first tick wrote %s = %v", k, v[k])
		}
	}
	for k := range v {
		if strings.HasPrefix(k, "off/") {
			t.Errorf("a stopped guest was sampled: %s", k)
		}
	}
	for _, row := range first {
		if row.ServerID != host1 || row.At != "2026-11-15 09:00:00.000" {
			t.Fatalf("row %+v", row)
		}
		if row.GuestID == "101" && row.Node != "n2" || row.GuestID == "web" && row.Node != "n1" {
			t.Fatalf("node %+v", row)
		}
	}

	// 30 seconds later: rates from the counters.
	r.now = r.now.Add(30 * time.Second)
	m.guests[0].CPUSeconds, m.guests[0].NetIn, m.guests[0].NetOut = hv.Ptr(130.0), hv.Ptr[int64](4000), hv.Ptr[int64](300)
	m.guests[1].NetIn = hv.Ptr[int64](650)
	m.guests[2].CPUSeconds = hv.Ptr(130.0)
	r.s.Tick(context.Background())
	v = values(r.ch.take())
	if v["web/cpu"] != 0.5 || v["web/net_in_bytes_per_second"] != 100 || v["web/net_out_bytes_per_second"] != 10 || v["101/net_in_bytes_per_second"] != 20 || v["all/cpu"] != 0.5 {
		t.Fatalf("second tick: %v", v)
	}

	// The guest restarted: its counters went back, so that step has no rate; the next one does.
	r.now = r.now.Add(30 * time.Second)
	m.guests[0].CPUSeconds, m.guests[0].NetIn = hv.Ptr(1.0), hv.Ptr[int64](10)
	r.s.Tick(context.Background())
	v = values(r.ch.take())
	if _, ok := v["web/cpu"]; ok {
		t.Fatalf("a counter that went back made a rate: %v", v)
	}
	if _, ok := v["web/net_in_bytes_per_second"]; ok {
		t.Fatalf("a counter that went back made a rate: %v", v)
	}
	r.now = r.now.Add(30 * time.Second)
	m.guests[0].CPUSeconds, m.guests[0].NetIn = hv.Ptr(16.0), hv.Ptr[int64](310)
	r.s.Tick(context.Background())
	if v = values(r.ch.take()); v["web/cpu"] != 0.25 || v["web/net_in_bytes_per_second"] != 10 {
		t.Fatalf("after the restart: %v", v)
	}
	if st := r.s.Status(); st.State != "sampling" || st.LastRows == 0 || !st.LastAt.Equal(r.now) || st.IntervalSeconds != 30 {
		t.Fatalf("status %+v", st)
	}
}

func TestOneSlowHostDoesNotStallTheOthers(t *testing.T) {
	fast := &meter{host: hv.HostMetrics{Node: "fast", CPU: hv.Ptr(0.1)}}
	slow := &meter{block: true}
	broken := Target{ServerID: "33333333-3333-4333-8333-333333333333", Open: func() (hv.Driver, error) { return nil, errors.New("no secret") }}
	bare := Target{ServerID: "44444444-4444-4444-8444-444444444444", Open: func() (hv.Driver, error) { return nil, hv.ErrUnsupported }}
	r := newRig(t, target("22222222-2222-4222-8222-222222222222", slow), target(host1, fast), broken, bare)
	r.s.HostTimeout = 200 * time.Millisecond
	start := time.Now()
	r.s.Tick(context.Background())
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the tick waited %v for a slow host", took)
	}
	rows := r.ch.take()
	if len(rows) != 1 || rows[0].ServerID != host1 || rows[0].Metric != CPU {
		t.Fatalf("rows %+v", rows)
	}
	st := r.s.Status()
	if st.State != "sampling" || len(st.Failed) != 2 || !strings.HasPrefix(st.Failed[0], "22222222") || !strings.Contains(st.Failed[1], "no secret") {
		t.Fatalf("status %+v", st)
	}
}

func TestSamplerIdlesAndSaysWhy(t *testing.T) {
	m := &meter{host: hv.HostMetrics{CPU: hv.Ptr(0.1)}, guests: []hv.GuestReading{{ID: "g", Status: "Running", NetIn: hv.Ptr[int64](10)}}}
	r := newRig(t, target(host1, m))
	r.s.Tick(context.Background())
	r.ch.take()

	r.store = nil
	r.s.Tick(context.Background())
	if st := r.s.Status(); st.State != "idle" || !strings.Contains(st.Reason, "ClickHouse is not connected") {
		t.Fatalf("without ClickHouse: %+v", st)
	}
	if r.lead.holding || r.lead.released != 1 {
		t.Fatalf("an idle sampler must give up the lead: %+v", r.lead)
	}

	// Back on: the counter read before the pause makes no rate across it.
	r.store = r.ch.serve(t)
	r.now = r.now.Add(30 * time.Second)
	m.guests[0].NetIn = hv.Ptr[int64](40)
	r.s.Tick(context.Background())
	if v := values(r.ch.take()); len(v) != 1 || v["/cpu"] != 0.1 {
		t.Fatalf("after the pause: %v", v)
	}

	r.cfg.Enabled = false
	r.s.Tick(context.Background())
	if st := r.s.Status(); st.State != "off" || !strings.Contains(st.Reason, "turned off") || len(r.ch.take()) != 0 {
		t.Fatalf("turned off: %+v", st)
	}

	r.cfg.Enabled, r.lead.free = true, false
	r.s.Tick(context.Background())
	if st := r.s.Status(); st.State != "standby" || len(r.ch.take()) != 0 {
		t.Fatalf("without the lead: %+v", st)
	}

	r.lead.free = true
	r.ch.fail = true
	r.s.Tick(context.Background())
	if st := r.s.Status(); st.State != "idle" || !strings.Contains(st.Reason, "Writing samples to ClickHouse failed") || strings.Contains(st.Reason, "secret") {
		t.Fatalf("a refused insert: %+v", st)
	}
}

func TestKeepDaysChangesTheTTLOnce(t *testing.T) {
	r := newRig(t)
	r.s.Tick(context.Background())
	if len(r.ch.alters) != 0 {
		t.Fatalf("the TTL already matched: %v", r.ch.alters)
	}
	r.cfg.KeepDays = 7
	r.s.Tick(context.Background())
	r.s.Tick(context.Background())
	if len(r.ch.alters) != 1 || r.ch.alters[0] != "ALTER TABLE shippin_metric_samples MODIFY TTL toDateTime(at) + INTERVAL 7 DAY DELETE" {
		t.Fatalf("alters %q", r.ch.alters)
	}
	r.cfg.KeepDays = 0 // out of bounds: the default stands in
	r.ch.ttl = 7
	r.s.Tick(context.Background())
	if len(r.ch.alters) != 2 || !strings.Contains(r.ch.alters[1], "INTERVAL 30 DAY") {
		t.Fatalf("alters %q", r.ch.alters)
	}
}

func TestSettingsSectionDescribesItself(t *testing.T) {
	if err := Settings.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Config{{IntervalSeconds: 5, KeepDays: 30}, {IntervalSeconds: 30, KeepDays: 0}, {IntervalSeconds: 4000, KeepDays: 30}, {IntervalSeconds: 30, KeepDays: 400}} {
		if bad.Check() == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if Defaults.Check() != nil {
		t.Fatal("the defaults are out of bounds")
	}
}
