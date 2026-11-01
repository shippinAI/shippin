package main

import (
	"fmt"
	"html/template"
	"math"
	"strings"
)

type HealthState string

const (
	HealthOK       HealthState = "ok"
	HealthWarn     HealthState = "warn"
	HealthCritical HealthState = "critical"
	HealthUnknown  HealthState = "unknown"
)

type DashboardTab struct {
	ID     string
	Label  string
	Active bool
	Count  int
}

type ChartPerspective struct {
	ID          string
	Label       string
	Description string
	Active      bool
	SVG         template.HTML
	Legend      []string
}

type HealthWidget struct {
	ID          string
	Label       string
	Value       string
	Unit        string
	State       HealthState
	Message     string
	Tags        []string
	DrilldownID string
	SVG         template.HTML
}

type HostNode struct {
	ID          string
	Name        string
	Kind        string
	Scope       string
	Status      HealthState
	Address     string
	Tags        []string
	Connections []HostConnection
	Metrics     []HealthWidget
}

type HostConnection struct {
	Label  string
	Target string
	State  HealthState
}

func buildDashboardTabs(hosts []HostNode) []DashboardTab {
	internal := 0
	customer := 0
	for _, host := range hosts {
		switch host.Scope {
		case "customer":
			customer++
		default:
			internal++
		}
	}
	return []DashboardTab{
		{ID: "overview", Label: "Overview", Active: true, Count: len(hosts)},
		{ID: "internal", Label: "WardenClyffe-Infra", Count: internal},
		{ID: "customer", Label: "Customer", Count: customer},
		{ID: "connections", Label: "Connections", Count: countHostConnections(hosts)},
	}
}

func buildChartPerspectives(stats SystemStats, hosts []HostNode) []ChartPerspective {
	cpu := clampPercent(stats.CPU.UsagePerc)
	mem := clampPercent(stats.Memory.UsedPerc)
	disk := clampPercent(dashboardDiskPerc(stats.Disks))
	networkScore := 0.0
	if len(stats.Network) > 0 {
		networkScore = math.Min(100, float64(len(stats.Network))*14)
	}
	return []ChartPerspective{
		{
			ID:          "overview",
			Label:       "Stark Overview",
			Description: "A high-contrast system silhouette for fast operator scanning.",
			Active:      true,
			SVG:         renderStarkOverviewSVG(cpu, mem, disk, networkScore),
			Legend:      []string{"cpu", "memory", "storage", "network"},
		},
		{
			ID:          "capacity",
			Label:       "Capacity Bands",
			Description: "Resource pressure as boring normalized bands.",
			SVG:         renderCapacityBandsSVG(cpu, mem, disk),
			Legend:      []string{"compute", "ram", "disk"},
		},
		{
			ID:          "topology",
			Label:       "Provider Boundaries",
			Description: "Provider X -> host -> Clyffe tenant -> CT/VM with VLAN/tag segmentation.",
			SVG:         renderTopologySVG(hosts),
			Legend:      []string{"provider-x", "host-layer", "clyffe", "ct/vm"},
		},
	}
}

func buildHostNodes(stats SystemStats) []HostNode {
	graph := getInfrastructureGraph(stats)
	nodes := graphToDashboardNodes(graph, stats)
	if len(nodes) > 0 {
		nodes[0].Metrics = buildHealthWidgets(stats)
	}
	return nodes
}

func buildHealthWidgets(stats SystemStats) []HealthWidget {
	diskPerc := dashboardDiskPerc(stats.Disks)
	widgets := []HealthWidget{
		newGaugeWidget("cpu", "CPU", stats.CPU.UsagePerc, "%", healthForPercent(stats.CPU.UsagePerc), fmt.Sprintf("%d cores observed", stats.CPU.Cores), []string{"host", "compute"}),
		newGaugeWidget("memory", "Memory", stats.Memory.UsedPerc, "%", healthForPercent(stats.Memory.UsedPerc), fmt.Sprintf("%s used of %s", formatBytes(stats.Memory.Used), formatBytes(stats.Memory.Total)), []string{"host", "ram"}),
		newGaugeWidget("disk", "Storage", diskPerc, "%", healthForPercent(diskPerc), fmt.Sprintf("%s used of %s", formatBytes(dashboardDiskUsed(stats.Disks)), formatBytes(dashboardDiskTotal(stats.Disks))), []string{"host", "storage"}),
		newStatusWidget("network", "Network", stringState(len(stats.Network) > 0), fmt.Sprintf("%d interfaces detected", len(stats.Network)), []string{"host", "network"}),
		newStatusWidget("sessions", "Access", stringState(len(stats.LoggedIn) >= 0), fmt.Sprintf("%d active login sessions", len(stats.LoggedIn)), []string{"auth", "sessions"}),
		newStatusWidget("packages", "Updates", packageState(stats.PackageUpdates), packageMessage(stats.PackageUpdates), []string{"maintenance", "packages"}),
	}
	return widgets
}

func newGaugeWidget(id, label string, pct float64, unit string, state HealthState, message string, tags []string) HealthWidget {
	return HealthWidget{
		ID:          id,
		Label:       label,
		Value:       fmt.Sprintf("%.0f", pct),
		Unit:        unit,
		State:       state,
		Message:     message,
		Tags:        tags,
		DrilldownID: "drilldown-" + id,
		SVG:         renderGaugeSVG(pct, state),
	}
}

func newStatusWidget(id, label string, state HealthState, message string, tags []string) HealthWidget {
	return HealthWidget{
		ID:          id,
		Label:       label,
		Value:       stateLabel(state),
		State:       state,
		Message:     message,
		Tags:        tags,
		DrilldownID: "drilldown-" + id,
		SVG:         renderStatusSVG(state),
	}
}

func renderGaugeSVG(pct float64, state HealthState) template.HTML {
	pct = math.Max(0, math.Min(100, pct))
	return template.HTML(fmt.Sprintf(`<svg class="hw-svg" viewBox="0 0 120 120" role="img" aria-hidden="true">
  <defs>
    <linearGradient id="hw-grad-%s" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%%" class="hw-grad-a"/>
      <stop offset="100%%" class="hw-grad-b"/>
    </linearGradient>
  </defs>
  <circle class="hw-orbit" cx="60" cy="60" r="46"/>
  <g transform="rotate(-90 60 60)">
    <circle class="hw-track" cx="60" cy="60" r="42" pathLength="100"/>
    <circle class="hw-fill hw-fill-%s" cx="60" cy="60" r="42" pathLength="100" stroke-dasharray="%.1f 100"/>
  </g>
  <circle class="hw-core hw-core-%s" cx="60" cy="60" r="25"/>
</svg>`, state, state, pct, state))
}

func renderStatusSVG(state HealthState) template.HTML {
	return template.HTML(fmt.Sprintf(`<svg class="hw-svg" viewBox="0 0 120 120" role="img" aria-hidden="true">
  <circle class="hw-orbit" cx="60" cy="60" r="46"/>
  <path class="hw-pulse hw-fill-%s" d="M24 64 C36 38, 48 82, 60 58 S84 42, 96 62"/>
  <circle class="hw-node hw-core-%s" cx="60" cy="60" r="13"/>
</svg>`, state, state))
}

func renderStarkOverviewSVG(cpu, mem, disk, network float64) template.HTML {
	points := []struct {
		label string
		x     float64
		y     float64
		value float64
	}{
		{"CPU", 100, 36, cpu},
		{"RAM", 164, 100, mem},
		{"DSK", 100, 164, disk},
		{"NET", 36, 100, network},
	}
	var b strings.Builder
	b.WriteString(`<svg class="main-chart-svg" viewBox="0 0 200 200" role="img" aria-label="Warden system overview">`)
	b.WriteString(`<defs><radialGradient id="chartGlow" cx="50%" cy="50%" r="60%"><stop offset="0%" stop-color="rgba(108,138,255,.34)"/><stop offset="100%" stop-color="rgba(108,138,255,0)"/></radialGradient></defs>`)
	b.WriteString(`<rect class="main-chart-field" x="8" y="8" width="184" height="184" rx="28"/>`)
	b.WriteString(`<circle cx="100" cy="100" r="72" fill="url(#chartGlow)"/>`)
	b.WriteString(`<path class="main-chart-axis" d="M100 28 V172 M28 100 H172 M49 49 L151 151 M151 49 L49 151"/>`)
	b.WriteString(`<polygon class="main-chart-poly" points="`)
	for _, p := range points {
		scale := 0.28 + (p.value/100)*0.72
		x := 100 + (p.x-100)*scale
		y := 100 + (p.y-100)*scale
		b.WriteString(fmt.Sprintf("%.1f,%.1f ", x, y))
	}
	b.WriteString(`"/>`)
	for _, p := range points {
		b.WriteString(fmt.Sprintf(`<g><circle class="main-chart-node main-chart-node-%s" cx="%.1f" cy="%.1f" r="5"/><text class="main-chart-label" x="%.1f" y="%.1f">%s %.0f%%</text></g>`, healthForPercent(p.value), p.x, p.y, p.x, p.y-10, p.label, p.value))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

func renderCapacityBandsSVG(cpu, mem, disk float64) template.HTML {
	bands := []struct {
		label string
		value float64
		state HealthState
	}{
		{"compute", cpu, healthForPercent(cpu)},
		{"memory", mem, healthForPercent(mem)},
		{"storage", disk, healthForPercent(disk)},
	}
	var b strings.Builder
	b.WriteString(`<svg class="main-chart-svg" viewBox="0 0 200 200" role="img" aria-label="Capacity bands">`)
	b.WriteString(`<rect class="main-chart-field" x="8" y="8" width="184" height="184" rx="28"/>`)
	for i, band := range bands {
		y := 48 + i*46
		w := 124 * clampPercent(band.value) / 100
		b.WriteString(fmt.Sprintf(`<text class="main-chart-label" x="30" y="%d">%s</text>`, y-10, band.label))
		b.WriteString(fmt.Sprintf(`<rect class="band-track" x="30" y="%d" width="140" height="16" rx="8"/>`, y))
		b.WriteString(fmt.Sprintf(`<rect class="band-fill band-fill-%s" x="30" y="%d" width="%.1f" height="16" rx="8"/>`, band.state, y, w))
		b.WriteString(fmt.Sprintf(`<text class="band-value" x="174" y="%d">%.0f%%</text>`, y+12, band.value))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

func renderTopologySVG(hosts []HostNode) template.HTML {
	var b strings.Builder
	b.WriteString(`<svg class="main-chart-svg" viewBox="0 0 200 200" role="img" aria-label="Host topology">`)
	b.WriteString(`<rect class="main-chart-field" x="8" y="8" width="184" height="184" rx="28"/>`)
	b.WriteString(`<text class="topology-small" x="16" y="22">Provider X</text>`)
	b.WriteString(`<text class="topology-small" x="62" y="22">Hosts</text>`)
	b.WriteString(`<text class="topology-small" x="114" y="22">Clyffe</text>`)
	b.WriteString(`<text class="topology-small" x="162" y="22">CT/VM</text>`)

	providers := filterByTag(hosts, "layer-provider")
	hostLayer := filterByTag(hosts, "layer-host")
	customers := filterByTag(hosts, "layer-customer")
	resources := filterByTag(hosts, "layer-resource")

	drawColumn := func(nodes []HostNode, x float64, radius float64) {
		if len(nodes) == 0 {
			return
		}
		for i, n := range nodes {
			y := 42.0 + float64(i)*(140.0/float64(hwMax(1, len(nodes)-1)))
			b.WriteString(fmt.Sprintf(`<circle class="topology-node hw-core-%s" cx="%.1f" cy="%.1f" r="%.1f"/>`, n.Status, x, y, radius))
			lbl := n.Name
			if len(lbl) > 11 {
				lbl = lbl[:11] + "…"
			}
			b.WriteString(fmt.Sprintf(`<text class="topology-small" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`, x, y+13, lbl))
		}
	}

	drawColumn(providers, 28, 8)
	drawColumn(hostLayer, 74, 6)
	drawColumn(customers, 126, 6)
	drawColumn(resources, 172, 5)

	for i := 0; i < hwMin(len(providers), len(hostLayer)); i++ {
		y1 := 42.0 + float64(i)*(140.0/float64(hwMax(1, len(providers)-1)))
		y2 := 42.0 + float64(i)*(140.0/float64(hwMax(1, len(hostLayer)-1)))
		b.WriteString(fmt.Sprintf(`<path class="topology-link" d="M28 %.1f L74 %.1f"/>`, y1, y2))
	}
	for i := 0; i < hwMin(len(hostLayer), len(customers)); i++ {
		y1 := 42.0 + float64(i)*(140.0/float64(hwMax(1, len(hostLayer)-1)))
		y2 := 42.0 + float64(i)*(140.0/float64(hwMax(1, len(customers)-1)))
		b.WriteString(fmt.Sprintf(`<path class="topology-link" d="M74 %.1f L126 %.1f"/>`, y1, y2))
	}
	for i := 0; i < hwMin(len(customers), len(resources)); i++ {
		y1 := 42.0 + float64(i)*(140.0/float64(hwMax(1, len(customers)-1)))
		y2 := 42.0 + float64(i)*(140.0/float64(hwMax(1, len(resources)-1)))
		b.WriteString(fmt.Sprintf(`<path class="topology-link" d="M126 %.1f L172 %.1f"/>`, y1, y2))
	}

	b.WriteString(`<circle class="topology-core" cx="28" cy="32" r="5"/><text class="topology-label" x="28" y="35" text-anchor="middle">X</text>`)
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

func healthForPercent(pct float64) HealthState {
	switch {
	case pct >= 90:
		return HealthCritical
	case pct >= 75:
		return HealthWarn
	default:
		return HealthOK
	}
}

func clampPercent(pct float64) float64 {
	return math.Max(0, math.Min(100, pct))
}

func aggregateHealth(stats SystemStats) HealthState {
	state := HealthOK
	for _, widget := range buildHealthWidgets(stats) {
		if widget.State == HealthCritical {
			return HealthCritical
		}
		if widget.State == HealthWarn {
			state = HealthWarn
		}
	}
	return state
}

func packageState(pkg PackageInfo) HealthState {
	if pkg.Security > 0 {
		return HealthCritical
	}
	if pkg.Total > 0 {
		return HealthWarn
	}
	return HealthOK
}

func packageMessage(pkg PackageInfo) string {
	if pkg.Security > 0 {
		return fmt.Sprintf("%d updates, %d security", pkg.Total, pkg.Security)
	}
	if pkg.Total > 0 {
		return fmt.Sprintf("%d updates available", pkg.Total)
	}
	return "system packages look current"
}

func stringState(ok bool) HealthState {
	if ok {
		return HealthOK
	}
	return HealthUnknown
}

func stateLabel(state HealthState) string {
	switch state {
	case HealthOK:
		return "Healthy"
	case HealthWarn:
		return "Watch"
	case HealthCritical:
		return "Action"
	default:
		return "Unknown"
	}
}

func countHostConnections(hosts []HostNode) int {
	total := 0
	for _, host := range hosts {
		total += len(host.Connections)
	}
	return total
}

func filterByTag(hosts []HostNode, tag string) []HostNode {
	out := make([]HostNode, 0)
	for _, h := range hosts {
		for _, t := range h.Tags {
			if t == tag {
				out = append(out, h)
				break
			}
		}
	}
	return out
}

func hwMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func hwMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func dashboardDiskTotal(disks []DiskInfo) uint64 {
	var total uint64
	for _, disk := range disks {
		total += disk.Total
	}
	return total
}

func dashboardDiskUsed(disks []DiskInfo) uint64 {
	var used uint64
	for _, disk := range disks {
		used += disk.Used
	}
	return used
}

func dashboardDiskPerc(disks []DiskInfo) float64 {
	total := dashboardDiskTotal(disks)
	if total == 0 {
		return 0
	}
	return float64(dashboardDiskUsed(disks)) / float64(total) * 100
}

func joinTags(tags []string) string {
	return strings.Join(tags, " ")
}
