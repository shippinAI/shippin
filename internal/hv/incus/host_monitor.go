package incus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"shippin.cloud/console/internal/hv"
)

// Incus answers /1.0/resources from what it read when its daemon started, so that call cannot say how
// busy the host is now. The host monitor is how the panel finds out without installing anything on the
// host: one small container, made through the same API, that sees the host's /proc read-only. The panel
// runs one short recorded command in it per sample and reads the output back. Nothing is made unless
// someone turns the monitor on, and turning it off removes the container.

// MonitorName is the host monitor's instance name.
const MonitorName = "shippin-host-monitor"

// monitorRole marks the instance as the panel's own, so only that one is ever removed as the monitor.
const (
	monitorRoleKey = "user.shippin.role"
	monitorRole    = "host-monitor"
	monitorProc    = "/host/proc"
)

// monitorCommand prints the load average, the CPU counters twice a second apart, and memory.
const monitorCommand = "cat " + monitorProc + "/loadavg; head -n1 " + monitorProc + "/stat; sleep 1; head -n1 " + monitorProc +
	"/stat; grep -E '^(MemTotal|MemAvailable):' " + monitorProc + "/meminfo"

// HostMonitor reports whether the monitor exists on this server and whether it is running.
func (c *Client) HostMonitor(ctx context.Context) (hv.HostMonitorState, error) {
	body, code, err := c.do(ctx, http.MethodGet, c.path(instancePath(MonitorName)), nil)
	if err != nil {
		return hv.HostMonitorState{}, err
	}
	if code == http.StatusNotFound {
		return hv.HostMonitorState{}, nil
	}
	if code != http.StatusOK {
		return hv.HostMonitorState{}, fmt.Errorf("incus: host monitor HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Metadata struct {
			Status string            `json:"status"`
			Config map[string]string `json:"config"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return hv.HostMonitorState{}, err
	}
	if r.Metadata.Config[monitorRoleKey] != monitorRole {
		return hv.HostMonitorState{}, fmt.Errorf("incus: an instance named %s exists and is not the panel's monitor; rename or remove it first", MonitorName)
	}
	return hv.HostMonitorState{Enabled: true, Guest: MonitorName, Running: strings.EqualFold(r.Metadata.Status, "running")}, nil
}

// EnableHostMonitor makes the monitor from an image and starts it; one that exists is only started.
// It has no network, one CPU and little memory, and starts with the host.
func (c *Client) EnableHostMonitor(ctx context.Context, image hv.GuestSpec) (hv.HostMonitorState, error) {
	st, err := c.HostMonitor(ctx)
	if err != nil {
		return st, err
	}
	if !st.Enabled {
		source, err := imageSource(image)
		if err != nil {
			return st, err
		}
		raw, err := json.Marshal(map[string]any{"name": MonitorName, "type": "container", "source": source,
			"config": map[string]string{monitorRoleKey: monitorRole, "limits.memory": "64MiB", "limits.cpu": "1", "boot.autostart": "true"},
			"devices": map[string]any{
				"hostproc": map[string]string{"type": "disk", "source": "/proc", "path": monitorProc, "readonly": "true"},
				// It reads files and talks to nobody, so the profile's network is left out.
				"eth0": map[string]string{"type": "none"},
			}})
		if err != nil {
			return st, err
		}
		body, code, err := c.do(ctx, http.MethodPost, c.path("/1.0/instances"), raw)
		if err != nil {
			return st, err
		}
		if code != http.StatusOK && code != http.StatusAccepted && code != http.StatusCreated {
			return st, fmt.Errorf("incus: host monitor HTTP %d: %s", code, clip(body))
		}
		if err := c.wait(ctx, body); err != nil {
			return st, err
		}
	}
	if !st.Running {
		if _, err := c.Start(ctx, MonitorName); err != nil {
			return hv.HostMonitorState{Enabled: true, Guest: MonitorName}, fmt.Errorf("the host monitor was made but did not start: %w", err)
		}
	}
	return c.HostMonitor(ctx)
}

// DisableHostMonitor removes the monitor. A server without one is left as it is.
func (c *Client) DisableHostMonitor(ctx context.Context) error {
	st, err := c.HostMonitor(ctx)
	if err != nil || !st.Enabled {
		return err
	}
	return c.DeleteGuest(ctx, MonitorName)
}

// errNoMonitor is a server with no running monitor: there are no live host readings to give.
var errNoMonitor = errors.New("incus: no host monitor")

// monitorReading runs the monitor's command once and reads what it printed.
func (c *Client) monitorReading(ctx context.Context) (string, error) {
	raw, _ := json.Marshal(map[string]any{"command": []string{"sh", "-c", monitorCommand}, "record-output": true, "wait-for-websocket": false, "interactive": false})
	body, code, err := c.do(ctx, http.MethodPost, c.path(instancePath(MonitorName)+"/exec"), raw)
	if err != nil {
		return "", err
	}
	// No such instance, or one that is stopped.
	if code == http.StatusNotFound || code == http.StatusBadRequest || code == http.StatusConflict {
		return "", errNoMonitor
	}
	if code != http.StatusOK && code != http.StatusAccepted {
		return "", fmt.Errorf("incus: host monitor HTTP %d: %s", code, clip(body))
	}
	var started struct {
		Metadata struct {
			ID string `json:"id"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &started) != nil || started.Metadata.ID == "" {
		return "", errors.New("incus: the host monitor's command did not start")
	}
	body, code, err = c.do(ctx, http.MethodGet, c.path("/1.0/operations/"+url.PathEscape(started.Metadata.ID)+"/wait", "timeout", "15"), nil)
	if err != nil {
		return "", err
	}
	var done struct {
		Metadata struct {
			Metadata struct {
				Output map[string]string `json:"output"`
				Return *int              `json:"return"`
			} `json:"metadata"`
		} `json:"metadata"`
	}
	if code != http.StatusOK || json.Unmarshal(body, &done) != nil {
		return "", fmt.Errorf("incus: host monitor HTTP %d: %s", code, clip(body))
	}
	out := done.Metadata.Metadata.Output
	// The recorded output is kept on the host until it is removed, so both files go whatever was read.
	defer func() {
		for _, p := range out {
			if strings.HasPrefix(p, instancePath(MonitorName)+"/logs/") {
				_, _, _ = c.do(ctx, http.MethodDelete, c.path(p), nil)
			}
		}
	}()
	if done.Metadata.Metadata.Return == nil || *done.Metadata.Metadata.Return != 0 || !strings.HasPrefix(out["1"], instancePath(MonitorName)+"/logs/") {
		return "", errors.New("incus: the host monitor's command failed")
	}
	body, code, err = c.do(ctx, http.MethodGet, c.path(out["1"]), nil)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("incus: host monitor output HTTP %d", code)
	}
	return string(body), nil
}

// liveHost fills m with what the monitor read: load, the busy share of all CPUs over one second, and
// memory in use, which is the total less what the kernel says is available.
func liveHost(m *hv.HostMetrics, out string) error {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 5 {
		return errors.New("incus: the host monitor printed too little")
	}
	load, err := strconv.ParseFloat(firstField(lines[0]), 64)
	if err != nil {
		return fmt.Errorf("incus: host load: %w", err)
	}
	busy1, all1, err := cpuCounters(lines[1])
	if err != nil {
		return err
	}
	busy2, all2, err := cpuCounters(lines[2])
	if err != nil {
		return err
	}
	var total, available int64
	for _, l := range lines[3:] {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		kb, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			return fmt.Errorf("incus: host memory: %w", err)
		}
		switch f[0] {
		case "MemTotal:":
			total = kb * 1024
		case "MemAvailable:":
			available = kb * 1024
		}
	}
	if total <= 0 || available <= 0 || available > total {
		return errors.New("incus: the host monitor gave no memory reading")
	}
	m.Load1 = hv.Ptr(load)
	if all2 > all1 && busy2 >= busy1 {
		m.CPU = hv.Ptr(float64(busy2-busy1) / float64(all2-all1))
	}
	m.MemoryUsed, m.MemoryTotal = hv.Ptr(total-available), hv.Ptr(total)
	return nil
}

func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

// cpuCounters reads the "cpu" line of /proc/stat: time not idle and not waiting for a disk, and all time.
func cpuCounters(line string) (busy, all uint64, err error) {
	f := strings.Fields(line)
	if len(f) < 6 || f[0] != "cpu" {
		return 0, 0, errors.New("incus: the host monitor gave no CPU reading")
	}
	for i, v := range f[1:] {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("incus: host CPU: %w", err)
		}
		// Guest time (fields 9 and 10) is already counted in user and nice.
		if i >= 8 {
			break
		}
		all += n
		// Fields 4 and 5 are idle and iowait.
		if i != 3 && i != 4 {
			busy += n
		}
	}
	return busy, all, nil
}
