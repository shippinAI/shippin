package hostd

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// HostStats is the read-only operator view of the Incus host.
type HostStats struct {
	CPUs      int            `json:"cpus"`
	CPUPct    float64        `json:"cpu_pct"`
	Load      [3]float64     `json:"load"`
	MemTotal  int64          `json:"mem_total"`
	MemAvail  int64          `json:"mem_available"`
	UptimeS   int64          `json:"uptime_s"`
	Pools     []ZPool        `json:"zfs_pools"`
	Instances map[string]int `json:"instances"`
	Projects  int            `json:"projects"`
	Ports     PortPool       `json:"ssh_ports"`
}

type ZPool struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Alloc  int64  `json:"alloc"`
	Free   int64  `json:"free"`
	Health string `json:"health"`
}

type PortPool struct {
	Min  int `json:"min"`
	Max  int `json:"max"`
	Used int `json:"used"`
}

func (s *Server) hostStats(w http.ResponseWriter, r *http.Request) {
	st := HostStats{CPUs: runtime.NumCPU(), Instances: map[string]int{}}
	st.CPUPct = cpuPct()
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		_, _ = fmt.Sscanf(string(b), "%f %f %f", &st.Load[0], &st.Load[1], &st.Load[2])
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		var u float64
		_, _ = fmt.Sscanf(string(b), "%f", &u)
		st.UptimeS = int64(u)
	}
	st.MemTotal, st.MemAvail = meminfo("MemTotal"), meminfo("MemAvailable")
	st.Pools = zpools(r.Context())
	var insts []struct {
		Status string `json:"status"`
	}
	if err := query(r.Context(), "/1.0/instances?all-projects=true&recursion=1", &insts); err == nil {
		for _, i := range insts {
			st.Instances[strings.ToLower(i.Status)]++
		}
	}
	var projs []string
	if query(r.Context(), "/1.0/projects", &projs) == nil {
		st.Projects = len(projs)
	}
	ps, _ := s.loadPorts()
	used := map[int]bool{}
	for _, p := range ps {
		if p.Port >= s.cfg.PortMin && p.Port <= s.cfg.PortMax {
			used[p.Port] = true
		}
	}
	st.Ports = PortPool{s.cfg.PortMin, s.cfg.PortMax, len(used)}
	reply(w, st)
}

func zpools(ctx context.Context) []ZPool {
	out, err := run(ctx, "", "zpool", "list", "-Hp", "-o", "name,size,alloc,free,health")
	pools := []ZPool{}
	if err != nil {
		return pools
	}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(l, "\t")
		if len(f) < 5 {
			continue
		}
		p := ZPool{Name: f[0], Health: f[4]}
		p.Size, _ = strconv.ParseInt(f[1], 10, 64)
		p.Alloc, _ = strconv.ParseInt(f[2], 10, 64)
		p.Free, _ = strconv.ParseInt(f[3], 10, 64)
		pools = append(pools, p)
	}
	return pools
}

func meminfo(key string) int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), ":"); ok && k == key {
			n, _ := strconv.ParseInt(strings.Fields(v)[0], 10, 64)
			return n * 1024
		}
	}
	return 0
}

// cpuPct samples /proc/stat twice, 250ms apart.
func cpuPct() float64 {
	a, ai := cpuTimes()
	time.Sleep(250 * time.Millisecond)
	b, bi := cpuTimes()
	if b <= a {
		return 0
	}
	return 100 * (1 - float64(bi-ai)/float64(b-a))
}

func cpuTimes() (total, idle int64) {
	raw, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0
	}
	line, _, _ := strings.Cut(string(raw), "\n")
	for i, f := range strings.Fields(line)[1:] {
		n, _ := strconv.ParseInt(f, 10, 64)
		total += n
		if i == 3 || i == 4 {
			idle += n
		}
	}
	return total, idle
}
