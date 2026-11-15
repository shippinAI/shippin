package metric

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"shippin.cloud/console/internal/clickhouse"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/settings"
)

// Target is one attached host the sampler reads.
type Target struct {
	ServerID string
	// Open builds the host's driver for one tick. A host whose kind has no driver (bare) is skipped.
	Open func() (hv.Driver, error)
}

// Leader elects one sampling panel process among several.
type Leader interface {
	// Hold keeps or takes the lead and reports whether this process has it.
	Hold(ctx context.Context) (bool, error)
	// Release gives it up, so another process can take it.
	Release()
}

// Sampler writes every attached host's and guest's readings to ClickHouse once per interval
// (OBS-01). Only the process holding Lead samples, so several panel processes write each reading
// once, and the previous reading of every counter, which a rate needs, lives in that one process.
type Sampler struct {
	Targets    func(ctx context.Context) ([]Target, error)
	ClickHouse func() *clickhouse.Client
	Config     func(ctx context.Context) Config
	Lead       Leader
	Now        func() time.Time
	// HostTimeout bounds one host's reads, so a slow host never holds up the rest. Zero is 10 seconds,
	// or the interval when that is shorter.
	HostTimeout time.Duration
	// Parallel is how many hosts are read at once; zero is 8.
	Parallel int

	mu     sync.Mutex
	status ports.Sampling
	prev   map[counterKey]counter
	ttl    *clickhouse.Client // the store whose TTL was last set to ttlDay
	ttlDay int
}

type counterKey struct{ server, guest, name string }

type counter struct {
	value float64
	at    time.Time
}

// Sample is one row of the samples table.
type Sample struct {
	At       string  `json:"at"`
	ServerID string  `json:"server_id"`
	GuestID  string  `json:"guest_id"`
	Node     string  `json:"node"`
	Metric   string  `json:"metric"`
	Value    float64 `json:"value"`
}

// TimeFormat is how a sample's time is written to ClickHouse's DateTime64(3, 'UTC').
const TimeFormat = "2006-01-02 15:04:05.000"

// Status is what the sampler is doing now.
func (s *Sampler) Status() ports.Sampling {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.status
	if st.State == "" {
		st.State, st.Reason = "starting", "The sampler has not run yet."
	}
	st.Failed = append([]string(nil), st.Failed...)
	return st
}

// Run samples until ctx ends: once at the start, then every interval, and again whenever kick fires (a
// settings save). While sampling is off it looks at its settings again every settings.Fresh.
func (s *Sampler) Run(ctx context.Context, kick <-chan struct{}) {
	defer s.Lead.Release()
	for {
		cfg := s.Tick(ctx)
		wait := settings.Fresh
		if cfg.Enabled {
			wait = time.Duration(cfg.IntervalSeconds) * time.Second
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		case <-kick:
			t.Stop()
		}
	}
}

// config is the section, with the defaults standing in for a value out of bounds.
func (s *Sampler) config(ctx context.Context) Config {
	c := s.Config(ctx)
	if c.IntervalSeconds < MinInterval || c.IntervalSeconds > MaxInterval {
		c.IntervalSeconds = Defaults.IntervalSeconds
	}
	if c.KeepDays < MinKeepDays || c.KeepDays > MaxKeepDays {
		c.KeepDays = Defaults.KeepDays
	}
	return c
}

// Tick samples once if this process should, and returns the settings it ran with.
func (s *Sampler) Tick(ctx context.Context) Config {
	cfg := s.config(ctx)
	ch := s.ClickHouse()
	if ch != nil {
		s.keep(ctx, ch, cfg.KeepDays)
	}
	switch {
	case !cfg.Enabled:
		s.stand(cfg, "off", "Recording metrics is turned off in the Metrics settings.")
		return cfg
	case ch == nil:
		s.stand(cfg, "idle", "ClickHouse is not connected, so there is nowhere to keep samples.")
		return cfg
	}
	held, err := s.Lead.Hold(ctx)
	switch {
	case err != nil:
		s.stand(cfg, "idle", "The database could not be reached to choose which panel process samples.")
		return cfg
	case !held:
		s.stand(cfg, "standby", "Another panel process is recording metrics.")
		return cfg
	}
	rows, failed := s.collect(ctx, cfg)
	st := ports.Sampling{State: "sampling", IntervalSeconds: cfg.IntervalSeconds, LastRows: len(rows), Failed: failed}
	if err := ch.Insert(ctx, Table, rows); err != nil {
		log.Printf("metrics: writing %d samples: %v", len(rows), err)
		st.State, st.Reason, st.LastRows = "idle", "Writing samples to ClickHouse failed: "+clipErr(err), 0
	} else {
		st.LastAt = s.now()
	}
	s.mu.Lock()
	if st.LastAt.IsZero() {
		st.LastAt = s.status.LastAt
	}
	changed := s.status.State != st.State
	s.status = st
	s.mu.Unlock()
	if changed {
		log.Printf("metrics: %s every %ds", st.State, cfg.IntervalSeconds)
	}
	return cfg
}

// stand records why this process is not sampling, gives up the lead and forgets every counter, since
// a reading from before the pause must not make a rate across it.
func (s *Sampler) stand(cfg Config, state, reason string) {
	s.Lead.Release()
	s.mu.Lock()
	changed := s.status.State != state || s.status.Reason != reason
	s.status = ports.Sampling{State: state, Reason: reason, IntervalSeconds: cfg.IntervalSeconds, LastAt: s.status.LastAt}
	s.prev = nil
	s.mu.Unlock()
	if changed {
		log.Printf("metrics: %s: %s", state, reason)
	}
}

// keep applies keep days to the table's TTL once per change, from whichever process sees it.
func (s *Sampler) keep(ctx context.Context, ch *clickhouse.Client, days int) {
	s.mu.Lock()
	done := s.ttl == ch && s.ttlDay == days
	s.mu.Unlock()
	if done {
		return
	}
	changed, err := KeepFor(ctx, ch, days)
	if err != nil {
		log.Printf("metrics: keeping samples %d days: %v", days, err)
		return
	}
	if changed {
		log.Printf("metrics: samples are now kept %d days", days)
	}
	s.mu.Lock()
	s.ttl, s.ttlDay = ch, days
	s.mu.Unlock()
}

// reading is what one host gave in one tick.
type reading struct {
	target Target
	at     time.Time
	host   *hv.HostMetrics
	guests []hv.GuestReading
	err    error
}

// collect reads every host, each within its own timeout, and turns the readings into rows.
func (s *Sampler) collect(ctx context.Context, cfg Config) ([]any, []string) {
	targets, err := s.Targets(ctx)
	if err != nil {
		log.Printf("metrics: listing hosts: %v", err)
		return nil, []string{"the host list could not be read"}
	}
	timeout := s.HostTimeout
	if timeout <= 0 {
		timeout = min(10*time.Second, time.Duration(cfg.IntervalSeconds)*time.Second)
	}
	parallel := s.Parallel
	if parallel <= 0 {
		parallel = 8
	}
	readings := make([]reading, len(targets))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			hctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			readings[i] = s.read(hctx, t)
		}()
	}
	wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	next := map[counterKey]counter{}
	maxAge := max(10*time.Minute, 3*time.Duration(cfg.IntervalSeconds)*time.Second)
	rows := []any{}
	var failed []string
	read := map[string]bool{}
	for _, r := range readings {
		if errors.Is(r.err, hv.ErrUnsupported) {
			continue
		}
		if r.err != nil {
			failed = append(failed, r.target.ServerID+": "+clipErr(r.err))
		}
		if r.host == nil && r.guests == nil {
			continue
		}
		read[r.target.ServerID] = true
		rows = append(rows, s.rows(r, next, maxAge)...)
	}
	// A host that failed this tick keeps its counters, so its next good reading still makes a rate.
	for k, c := range s.prev {
		if !read[k.server] && s.now().Sub(c.at) <= maxAge {
			next[k] = c
		}
	}
	s.prev = next
	return rows, failed
}

// read is one host's readings. A host that is neither metered nor has guests to read is unsupported.
func (s *Sampler) read(ctx context.Context, t Target) reading {
	r := reading{target: t}
	d, err := t.Open()
	if err != nil {
		r.err = err
		return r
	}
	hm, hostOK := d.(hv.HostMeter)
	gm, guestsOK := d.(hv.GuestMeter)
	if !hostOK && !guestsOK {
		r.err = hv.ErrUnsupported
		return r
	}
	var errs []error
	if hostOK {
		if m, err := hm.HostMetrics(ctx); err == nil {
			r.host = &m
		} else {
			errs = append(errs, fmt.Errorf("host: %w", err))
		}
	}
	if guestsOK {
		if gs, err := gm.GuestReadings(ctx); err == nil {
			r.guests = gs
		} else {
			errs = append(errs, fmt.Errorf("guests: %w", err))
		}
	}
	r.at = s.now()
	// What one part could not read is reported; what the other read is still written.
	r.err = errors.Join(errs...)
	return r
}

// rows are one host's samples. next collects the counters read now, for the next tick's rates.
func (s *Sampler) rows(r reading, next map[counterKey]counter, maxAge time.Duration) []any {
	at := r.at.UTC().Format(TimeFormat)
	var out []any
	add := func(guest, node, metric string, v *float64) {
		if v != nil {
			out = append(out, Sample{At: at, ServerID: r.target.ServerID, GuestID: guest, Node: node, Metric: metric, Value: *v})
		}
	}
	rate := func(guest, name string, v *int64) *float64 {
		if v == nil {
			return nil
		}
		return s.rate(counterKey{r.target.ServerID, guest, name}, float64(*v), r.at, next, maxAge)
	}
	hostNode, hostCPUs := "", 0
	if h := r.host; h != nil {
		hostNode, hostCPUs = h.Node, h.CPUs
		add("", h.Node, CPU, fraction(h.CPU))
		add("", h.Node, Load1, h.Load1)
		add("", h.Node, MemoryUsed, f(h.MemoryUsed))
		add("", h.Node, MemoryTotal, f(h.MemoryTotal))
		add("", h.Node, DiskUsed, f(h.DiskUsed))
		add("", h.Node, DiskTotal, f(h.DiskTotal))
		add("", h.Node, NetIn, h.NetInRate)
		add("", h.Node, NetOut, h.NetOutRate)
	}
	for _, g := range r.guests {
		// A guest that isn't running has nothing to sample: its time is a gap, not zeros.
		if g.ID == "" || !strings.EqualFold(g.Status, "running") {
			continue
		}
		node := g.Node
		if node == "" {
			node = hostNode
		}
		cpu := fraction(g.CPU)
		if cpu == nil && g.CPUSeconds != nil {
			cpus := g.CPULimit
			if cpus == 0 {
				cpus = hostCPUs
			}
			busy := s.rate(counterKey{r.target.ServerID, g.ID, "cpu_seconds"}, *g.CPUSeconds, r.at, next, maxAge)
			if busy != nil && cpus > 0 {
				cpu = fraction(hv.Ptr(*busy / float64(cpus)))
			}
		}
		add(g.ID, node, CPU, cpu)
		add(g.ID, node, MemoryUsed, f(g.MemoryUsed))
		add(g.ID, node, MemoryTotal, f(g.MemoryTotal))
		add(g.ID, node, DiskUsed, f(g.DiskUsed))
		add(g.ID, node, DiskTotal, f(g.DiskTotal))
		add(g.ID, node, NetIn, rate(g.ID, "net_in", g.NetIn))
		add(g.ID, node, NetOut, rate(g.ID, "net_out", g.NetOut))
	}
	return out
}

// rate is the counter's growth per second since its previous reading. There is none for the first
// reading, for a previous one older than maxAge, and for a counter that went backwards (the guest or
// host restarted): those are gaps, not zeros.
func (s *Sampler) rate(k counterKey, v float64, at time.Time, next map[counterKey]counter, maxAge time.Duration) *float64 {
	next[k] = counter{v, at}
	p, ok := s.prev[k]
	if !ok {
		return nil
	}
	dt := at.Sub(p.at)
	if dt <= 0 || dt > maxAge || v < p.value {
		return nil
	}
	return hv.Ptr((v - p.value) / dt.Seconds())
}

func (s *Sampler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func f(v *int64) *float64 {
	if v == nil {
		return nil
	}
	return hv.Ptr(float64(*v))
}

// fraction keeps a CPU share within 0 to 1: the provider's clock and ours differ a little, so a
// fully busy machine can read just over 1.
func fraction(v *float64) *float64 {
	if v == nil {
		return nil
	}
	return hv.Ptr(min(max(*v, 0), 1))
}

func clipErr(err error) string {
	s := err.Error()
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// PGLeader is the lead held as a PostgreSQL session advisory lock on one pooled connection. The lock
// lasts as long as the connection: a process that dies or loses its database loses the lead, and
// another takes it on its next tick.
type PGLeader struct {
	Pool *pgxpool.Pool
	Key  int64

	mu   sync.Mutex
	conn *pgxpool.Conn
}

// Hold keeps the lead while its connection answers, or tries to take it.
func (l *PGLeader) Hold(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn != nil {
		if _, err := l.conn.Exec(ctx, "SELECT 1"); err == nil {
			return true, nil
		}
		// The session may be gone, or only slow: close it rather than return it to the pool, where a
		// lock it still held would be kept by whatever query borrowed it next.
		l.drop()
	}
	if l.Pool == nil {
		return false, errors.New("metric: no database")
	}
	conn, err := l.Pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, l.Key).Scan(&ok); err != nil || !ok {
		conn.Release()
		return false, err
	}
	l.conn = conn
	return true, nil
}

// Release unlocks and returns the connection.
func (l *PGLeader) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := l.conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, l.Key); err != nil {
		l.drop()
		return
	}
	l.conn.Release()
	l.conn = nil
}

// drop closes the lead's connection instead of pooling it; closing the session frees its lock.
func (l *PGLeader) drop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = l.conn.Hijack().Close(ctx)
	l.conn = nil
}
