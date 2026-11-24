package hostd

import (
	"math"
	"net/http"
	"sort"
	"strings"
)

// CostRates price resource use (USD). Zero fields fall back to defaultRates.
type CostRates struct {
	CPUHour      float64 `json:"cpu_hour"`       // per CPU-second-hour actually used
	MemGiBMonth  float64 `json:"mem_gib_month"`  // per GiB of memory in use, for a 730 h month
	DiskGiBMonth float64 `json:"disk_gib_month"` // per GiB of root disk in use
	EgressGiB    float64 `json:"egress_gib"`     // per GiB transmitted
}

var defaultRates = CostRates{CPUHour: 0.02, MemGiBMonth: 2.5, DiskGiBMonth: 0.08, EgressGiB: 0.01}

func (c CostRates) withDefaults() CostRates {
	pick := func(v, d float64) float64 {
		if v > 0 {
			return v
		}
		return d
	}
	return CostRates{pick(c.CPUHour, defaultRates.CPUHour), pick(c.MemGiBMonth, defaultRates.MemGiBMonth),
		pick(c.DiskGiBMonth, defaultRates.DiskGiBMonth), pick(c.EgressGiB, defaultRates.EgressGiB)}
}

// ProjectUsage is one Shippin project's (or the tenant VM's) totals and cost.
type ProjectUsage struct {
	ProjectID  string  `json:"project_id"` // Shippin project UUID; "" = tenant-level (VM, legacy)
	Instances  []Usage `json:"instances"`
	CPUSec     float64 `json:"cpu_seconds"`
	MemBytes   float64 `json:"memory_bytes"`
	DiskBytes  float64 `json:"disk_bytes"`
	TxBytes    float64 `json:"net_tx_bytes"`
	AccruedUSD float64 `json:"accrued_usd"` // CPU used + egress since boot
	MonthlyUSD float64 `json:"monthly_usd"` // run rate: memory + disk held for a month
}

const gib = 1 << 30

func cents(v float64) float64 { return math.Round(v*100) / 100 }

// Rollup groups instance usage by Shippin project (inner Incus project pj-<uuid>) and prices it.
func Rollup(us []Usage, r CostRates) []ProjectUsage {
	r = r.withDefaults()
	by := map[string]*ProjectUsage{}
	for _, u := range us {
		id, _ := strings.CutPrefix(u.Project, "pj-")
		if id == u.Project {
			id = ""
		}
		p := by[id]
		if p == nil {
			p = &ProjectUsage{ProjectID: id}
			by[id] = p
		}
		p.Instances = append(p.Instances, u)
		p.CPUSec += u.CPUSec
		p.MemBytes += u.MemBytes
		p.DiskBytes += u.DiskBytes
		p.TxBytes += u.TxBytes
	}
	out := []ProjectUsage{}
	for _, p := range by {
		p.AccruedUSD = cents(p.CPUSec/3600*r.CPUHour + p.TxBytes/gib*r.EgressGiB)
		p.MonthlyUSD = cents(p.MemBytes/gib*r.MemGiBMonth + p.DiskBytes/gib*r.DiskGiBMonth)
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProjectID < out[j].ProjectID })
	return out
}

// tenantMetrics: GET /tenants/{t}/metrics. VM tenants: inner Incus metrics (per project) plus the
// tenant VM itself from the host; legacy tenants: the host project. Read-only.
func (s *Server) tenantMetrics(w http.ResponseWriter, r *http.Request) {
	t := r.PathValue("t")
	ctx := r.Context()
	host, err := runner(ctx, "", "incus", "query", "/1.0/metrics?project="+Project(t))
	if err != nil {
		fail(w, http.StatusConflict, "metrics unavailable")
		return
	}
	us := ParseMetrics(host)
	if p, err := s.newPlace(ctx, t); err == nil && p.vm {
		if inner, err := p.do(ctx, "query", "/1.0/metrics"); err == nil {
			for _, u := range ParseMetrics(inner) {
				if u.Project == InnerProject || strings.HasPrefix(u.Project, "pj-") {
					us = append(us, u)
				}
			}
		}
	}
	reply(w, map[string]any{"projects": Rollup(us, s.cfg.Costs), "rates": s.cfg.Costs.withDefaults()})
}
