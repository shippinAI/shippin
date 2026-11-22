package metric

import (
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// Limits of one range read, so a chart can't ask ClickHouse for more than a page can draw.
const (
	MaxPoints  = 1500                // buckets per metric
	MaxSpan    = 92 * 24 * time.Hour // from to to
	MinStep    = 10 * time.Second
	maxMetrics = 8
)

// ListInput is one host's or one guest's metrics over a range, in buckets of step seconds.
type ListInput struct {
	// ServerID is the host. GuestID is one of its guests, as the hypervisor names it; empty is the host.
	ServerID string `query:"server_id" json:"-"`
	GuestID  string `query:"guest_id" json:"-"`
	// Metrics are metric names separated by commas (docs/metrics.md).
	Metrics string `query:"metrics" json:"-"`
	// From and To are RFC 3339 times; From is rounded down to a whole step and To is not included.
	From string `query:"from" json:"-"`
	To   string `query:"to" json:"-"`
	// Step is the bucket length in whole seconds.
	Step string `query:"step" json:"-"`
}

// Range is the buckets of each metric asked for.
type Range struct {
	ServerID    string    `json:"server_id"`
	GuestID     string    `json:"guest_id"`
	From        time.Time `json:"from"`
	To          time.Time `json:"to"`
	StepSeconds int       `json:"step_seconds"`
	// Times are the start of every bucket; each series has one bucket per time, in the same order.
	Times  []time.Time `json:"times"`
	Series []Series    `json:"series"`
	// Sampler is what the answering panel process's sampler is doing, so a chart can say why recent
	// time is missing.
	Sampler SamplerState `json:"sampler"`
}

// Series is one metric's buckets.
type Series struct {
	Metric string `json:"metric"`
	Unit   string `json:"unit"`
	// Buckets[i] covers Times[i] for one step. It is null when there were no samples then: a gap.
	Buckets []*Bucket `json:"buckets"`
}

// Bucket is the samples of one metric in one step.
type Bucket struct {
	Avg     float64 `json:"avg"`
	Max     float64 `json:"max"`
	Samples int     `json:"samples"`
}

// SamplerState is ports.Sampling as the panel shows it.
type SamplerState struct {
	// State is sampling, standby, idle, off or starting.
	State           string     `json:"state"`
	Reason          string     `json:"reason,omitempty"`
	IntervalSeconds int        `json:"interval_seconds"`
	LastSampledAt   *time.Time `json:"last_sampled_at"`
}

// List is shippin.infrastructure.metric.list: GET /v1/operator/infrastructure/metrics.
var List = mcpl.Def[ListInput, Range]{
	Spec: mcpl.Spec{Summary: "Read a host's or guest's CPU, memory, disk and network over a time range, averaged into steps.", Access: mcpl.Staff, Perm: "infra.read"},
	Run: func(c *mcpl.Call, in ListInput) (Range, error) {
		q, err := parse(in)
		if err != nil {
			return Range{}, err
		}
		m, err := mcpl.Need[ports.Metrics](c)
		if err != nil {
			return Range{}, err
		}
		ch := m.ClickHouse()
		if ch == nil {
			return Range{}, &mcpl.Error{Status: http.StatusServiceUnavailable, Message: "ClickHouse is not connected, so there is no metrics history."}
		}
		var known bool
		if err := c.DB.QueryRow(c.Ctx, `SELECT EXISTS (SELECT 1 FROM servers WHERE id = $1)`, q.server).Scan(&known); err != nil {
			return Range{}, err
		}
		if !known {
			return Range{}, mcpl.NotFound("There is no host with that ID.")
		}
		step := int64(q.step / time.Millisecond)
		var rows []struct {
			Metric  string  `json:"metric"`
			Bucket  int64   `json:"b"`
			Avg     float64 `json:"avg"`
			Max     float64 `json:"max"`
			Samples int     `json:"n"`
		}
		names := "[" + strings.Join(quoteAll(q.metrics), ",") + "]"
		if err := ch.Query(c.Ctx, `SELECT metric, intDiv(toUnixTimestamp64Milli(at) - {from:Int64}, {step:Int64}) AS b,
			avg(value) AS avg, max(value) AS max, toUInt32(count()) AS n
			FROM `+Table+`
			WHERE server_id = {server:UUID} AND guest_id = {guest:String} AND metric IN {metrics:Array(String)}
			  AND at >= fromUnixTimestamp64Milli({from:Int64}, 'UTC') AND at < fromUnixTimestamp64Milli({to:Int64}, 'UTC')
			GROUP BY metric, b ORDER BY metric, b`,
			map[string]string{"server": q.server, "guest": q.guest, "metrics": names,
				"from": strconv.FormatInt(q.from.UnixMilli(), 10), "to": strconv.FormatInt(q.to.UnixMilli(), 10),
				"step": strconv.FormatInt(step, 10)}, &rows); err != nil {
			return Range{}, err
		}
		out := Range{ServerID: q.server, GuestID: q.guest, From: q.from, To: q.to, StepSeconds: int(q.step / time.Second),
			Times: make([]time.Time, q.points), Series: make([]Series, len(q.metrics)), Sampler: state(m.Sampling())}
		for i := range out.Times {
			out.Times[i] = q.from.Add(time.Duration(i) * q.step)
		}
		at := map[string]int{}
		for i, name := range q.metrics {
			unit, _ := UnitOf(name)
			out.Series[i] = Series{Metric: name, Unit: unit, Buckets: make([]*Bucket, q.points)}
			at[name] = i
		}
		for _, r := range rows {
			i, ok := at[r.Metric]
			if !ok || r.Bucket < 0 || r.Bucket >= int64(q.points) || math.IsNaN(r.Avg) {
				continue
			}
			out.Series[i].Buckets[r.Bucket] = &Bucket{Avg: r.Avg, Max: r.Max, Samples: r.Samples}
		}
		return out, nil
	},
}

// query is a checked ListInput.
type query struct {
	server, guest string
	metrics       []string
	from, to      time.Time
	step          time.Duration
	points        int
}

func parse(in ListInput) (query, error) {
	q := query{guest: strings.TrimSpace(in.GuestID)}
	id, err := uuid.Parse(strings.TrimSpace(in.ServerID))
	if err != nil {
		return q, mcpl.Invalid("server_id must be a host's ID.")
	}
	q.server = id.String()
	if len(q.guest) > 200 {
		return q, mcpl.Invalid("guest_id is too long.")
	}
	for _, name := range strings.Split(in.Metrics, ",") {
		name = strings.TrimSpace(name)
		if name == "" || slices.Contains(q.metrics, name) {
			continue
		}
		if _, ok := UnitOf(name); !ok {
			return q, mcpl.Invalid("Unknown metric " + strconv.Quote(clip(name, 60)) + "; the metrics are " + allNames() + ".")
		}
		q.metrics = append(q.metrics, name)
	}
	if len(q.metrics) == 0 || len(q.metrics) > maxMetrics {
		return q, mcpl.Invalid("Name between 1 and " + strconv.Itoa(maxMetrics) + " metrics in metrics, separated by commas: " + allNames() + ".")
	}
	secs, err := strconv.Atoi(strings.TrimSpace(in.Step))
	if err != nil || time.Duration(secs)*time.Second < MinStep {
		return q, mcpl.Invalid("step must be a whole number of seconds, at least " + strconv.Itoa(int(MinStep/time.Second)) + ".")
	}
	q.step = time.Duration(secs) * time.Second
	if q.from, err = time.Parse(time.RFC3339, strings.TrimSpace(in.From)); err != nil {
		return q, mcpl.Invalid("from must be an RFC 3339 time, such as 2026-11-15T09:00:00Z.")
	}
	if q.to, err = time.Parse(time.RFC3339, strings.TrimSpace(in.To)); err != nil {
		return q, mcpl.Invalid("to must be an RFC 3339 time, such as 2026-11-15T10:00:00Z.")
	}
	q.from, q.to = q.from.UTC().Truncate(q.step), q.to.UTC()
	if !q.to.After(q.from) {
		return q, mcpl.Invalid("to must be after from.")
	}
	if q.to.Sub(q.from) > MaxSpan {
		return q, mcpl.Invalid("A range can be at most " + strconv.Itoa(int(MaxSpan/(24*time.Hour))) + " days long.")
	}
	q.points = int((q.to.Sub(q.from) + q.step - 1) / q.step)
	if q.points > MaxPoints {
		return q, mcpl.Invalid("That is " + strconv.Itoa(q.points) + " steps; at most " + strconv.Itoa(MaxPoints) +
			" fit in one range, so use a step of at least " + strconv.Itoa(int((q.to.Sub(q.from)/MaxPoints+time.Second-1)/time.Second)) + " seconds.")
	}
	return q, nil
}

func state(s ports.Sampling) SamplerState {
	out := SamplerState{State: s.State, Reason: s.Reason, IntervalSeconds: s.IntervalSeconds}
	if !s.LastAt.IsZero() {
		t := s.LastAt.UTC()
		out.LastSampledAt = &t
	}
	return out
}

func allNames() string {
	names := make([]string, len(Names))
	for i, n := range Names {
		names[i] = n.Name
	}
	return strings.Join(names, ", ")
}

// quoteAll writes names as ClickHouse string literals for an Array(String) parameter. The names were
// checked against Names, so they hold no quote.
func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "'" + n + "'"
	}
	return out
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
