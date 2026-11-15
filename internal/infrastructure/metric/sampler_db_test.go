package metric_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/clickhouse"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/metric"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

// Two panel processes share one PostgreSQL: only one holds the lead, and the other takes it once it
// is given up.
func TestOneProcessHoldsTheLead(t *testing.T) {
	pool := mcpltest.DB(t)
	ctx := context.Background()
	key := time.Now().UnixNano()
	a, b := &metric.PGLeader{Pool: pool, Key: key}, &metric.PGLeader{Pool: pool, Key: key}
	if ok, err := a.Hold(ctx); err != nil || !ok {
		t.Fatalf("a: %v %v", ok, err)
	}
	if ok, err := a.Hold(ctx); err != nil || !ok {
		t.Fatalf("a again: %v %v", ok, err)
	}
	if ok, err := b.Hold(ctx); err != nil || ok {
		t.Fatalf("b took a held lead: %v %v", ok, err)
	}
	a.Release()
	if ok, err := b.Hold(ctx); err != nil || !ok {
		t.Fatalf("b after a let go: %v %v", ok, err)
	}
	b.Release()
}

// A lead whose check fails (here a context already ended, as on a slow database) is closed, not put
// back in the pool still holding the lock, so another process can take it.
func TestALeadInDoubtIsClosedNotPooled(t *testing.T) {
	pool := mcpltest.DB(t)
	ctx := context.Background()
	key := time.Now().UnixNano()
	a, b := &metric.PGLeader{Pool: pool, Key: key}, &metric.PGLeader{Pool: pool, Key: key}
	if ok, err := a.Hold(ctx); err != nil || !ok {
		t.Fatalf("a: %v %v", ok, err)
	}
	done, cancel := context.WithCancel(ctx)
	cancel()
	if ok, _ := a.Hold(done); ok {
		t.Fatal("a kept a lead it could not check")
	}
	// Nothing may still hold the lock: a pooled session holding it would hand the lead to whichever
	// code borrowed that connection next, and the same session can take an advisory lock twice.
	var held int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND objid = ($1::bigint & 4294967295)::oid AND granted`, key).Scan(&held); err != nil || held != 0 {
		t.Fatalf("no advisory lock is left held: %d %v", held, err)
	}
	if ok, err := b.Hold(ctx); err != nil || !ok {
		t.Fatalf("b after a's lead was in doubt: %v %v", ok, err)
	}
	b.Release()
}

type hostOnly struct {
	hv.Driver
	cpu float64
	in  int64
}

func (h *hostOnly) HostMetrics(context.Context) (hv.HostMetrics, error) {
	return hv.HostMetrics{Node: "n1", CPUs: 2, CPU: hv.Ptr(h.cpu)}, nil
}

func (h *hostOnly) GuestReadings(context.Context) ([]hv.GuestReading, error) {
	return []hv.GuestReading{{ID: "g1", Status: "Running", CPUSeconds: hv.Ptr(float64(h.in)), CPULimit: 1, NetIn: hv.Ptr(h.in)}}, nil
}

// The sampler's rows land in the real ClickHouse table, under the real lead, and read back.
func TestSamplesLandInClickHouse(t *testing.T) {
	ch := realClickHouse(t)
	pool := mcpltest.DB(t)
	ctx := context.Background()
	server := uuid.NewString()
	d := &hostOnly{cpu: 0.4, in: 100}
	now := time.Now().UTC().Truncate(time.Second)
	s := &metric.Sampler{
		Targets: func(context.Context) ([]metric.Target, error) {
			return []metric.Target{{ServerID: server, Open: func() (hv.Driver, error) { return d, nil }}}, nil
		},
		ClickHouse: func() *clickhouse.Client { return ch },
		Config:     func(context.Context) metric.Config { return metric.Defaults },
		Lead:       &metric.PGLeader{Pool: pool, Key: time.Now().UnixNano()},
		Now:        func() time.Time { return now },
	}
	defer s.Lead.Release()
	s.Tick(ctx)
	now = now.Add(30 * time.Second)
	d.in = 130
	s.Tick(ctx)
	if st := s.Status(); st.State != "sampling" || st.LastRows != 3 {
		t.Fatalf("status %+v", st)
	}
	var rows []struct {
		Guest  string  `json:"guest_id"`
		Metric string  `json:"metric"`
		Value  float64 `json:"value"`
		Node   string  `json:"node"`
	}
	if err := ch.Query(ctx, "SELECT guest_id, metric, value, node FROM "+metric.Table+" WHERE server_id = {s:UUID} ORDER BY at, guest_id, metric",
		map[string]string{"s": server}, &rows); err != nil {
		t.Fatal(err)
	}
	// Tick one: the host's CPU only (no rate yet). Tick two: the host's CPU and the guest's two rates.
	if len(rows) != 4 || rows[0].Metric != "cpu" || rows[0].Guest != "" || rows[1].Guest != "" ||
		rows[2].Guest != "g1" || rows[2].Metric != "cpu" || rows[2].Value != 1 || rows[3].Metric != "net_in_bytes_per_second" || rows[3].Value != 1 || rows[3].Node != "n1" {
		t.Fatalf("rows %+v", rows)
	}
}
