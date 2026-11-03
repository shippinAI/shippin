package hostd

import (
	"bufio"
	"strconv"
	"strings"
)

// Usage is one instance's resource use from Incus /1.0/metrics (OpenMetrics text).
type Usage struct {
	Name      string  `json:"name"`
	Project   string  `json:"project"`
	Type      string  `json:"type"`
	CPUSec    float64 `json:"cpu_seconds"`
	MemBytes  float64 `json:"memory_bytes"`
	DiskBytes float64 `json:"disk_bytes"`
	RxBytes   float64 `json:"net_rx_bytes"`
	TxBytes   float64 `json:"net_tx_bytes"`
	Procs     float64 `json:"processes"`
	BootUnix  float64 `json:"boot_unix"`
	memTotal  float64
	memAvail  float64
}

// labels parses `k="v",k2="v2"` (values are Incus-generated; quotes inside are not expected).
func labels(s string) map[string]string {
	out := map[string]string{}
	for _, kv := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			out[strings.TrimSpace(k)] = strings.Trim(v, `"`)
		}
	}
	return out
}

// ParseMetrics folds Incus metrics into per-instance usage. Unknown series are ignored; loopback
// traffic and filesystems other than / and /home are left out so numbers match what a user is billed for.

// billedMount: the root disk plus the workspace home volume (B2 precheck on Gold: a devspace's
// h-<w> custom volume is reported as a second incus_filesystem_* series with mountpoint="/home").
func billedMount(m string) bool { return m == "/" || m == "/home" }
func ParseMetrics(text string) []Usage {
	by := map[string]*Usage{}
	order := []string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		open, close := strings.IndexByte(line, '{'), strings.LastIndexByte(line, '}')
		if open < 0 || close < open {
			continue
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(line[close+1:]), 64)
		if err != nil {
			continue
		}
		l := labels(line[open+1 : close])
		key := l["project"] + "/" + l["name"]
		if l["name"] == "" {
			continue
		}
		u := by[key]
		if u == nil {
			u = &Usage{Name: l["name"], Project: l["project"], Type: l["type"]}
			by[key] = u
			order = append(order, key)
		}
		add(u, line[:open], l, val)
	}
	out := make([]Usage, 0, len(order))
	for _, k := range order {
		u := by[k]
		if u.memTotal > 0 && u.memAvail > 0 {
			u.MemBytes = u.memTotal - u.memAvail
		}
		out = append(out, *u)
	}
	return out
}

func add(u *Usage, name string, l map[string]string, v float64) {
	switch name {
	case "incus_cpu_seconds_total":
		if l["mode"] != "idle" && l["mode"] != "iowait" {
			u.CPUSec += v
		}
	case "incus_memory_MemTotal_bytes":
		u.memTotal = v
	case "incus_memory_MemAvailable_bytes":
		u.memAvail = v
	case "incus_memory_RSS_bytes":
		if u.MemBytes == 0 {
			u.MemBytes = v
		}
	case "incus_filesystem_size_bytes":
		if billedMount(l["mountpoint"]) {
			u.DiskBytes += v
		}
	case "incus_filesystem_avail_bytes":
		if billedMount(l["mountpoint"]) {
			u.DiskBytes -= v
		}
	case "incus_network_receive_bytes_total":
		if l["device"] != "lo" {
			u.RxBytes += v
		}
	case "incus_network_transmit_bytes_total":
		if l["device"] != "lo" {
			u.TxBytes += v
		}
	case "incus_procs_total":
		u.Procs = v
	case "incus_boot_time_seconds":
		u.BootUnix = v
	}
}
