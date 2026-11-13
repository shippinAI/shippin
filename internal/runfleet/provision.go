package runfleet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Spec is what a provisioner would need to create a guest. It carries no
// GitHub registration token; those are not requested while creation is unimplemented.
type Spec struct {
	RunnerID    string
	Name        string
	Size        Size
	Repos       []string
	Account     string
	AccountKind string
}

// Machine is a guest that exists. Stub provisioners never return one.
type Machine struct {
	Kind   string `json:"kind,omitempty"`
	VMID   int    `json:"vmid,omitempty"`
	Host   string `json:"host,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// ProvisionOutcome is the honest result of asking a provider to create a guest.
type ProvisionOutcome struct {
	Implemented bool     `json:"implemented"`
	Skipped     bool     `json:"skipped,omitempty"`
	Provider    string   `json:"provider"`
	Message     string   `json:"message"`
	Machine     *Machine `json:"machine,omitempty"`
}

// Provisioner creates guests and probes the ones already recorded in the fleet file.
type Provisioner interface {
	Name() string
	Provision(ctx context.Context, spec Spec) (Machine, error)
	Health(ctx context.Context, runner Runner) (substrate, hostLoad Signal)
}

// NotImplementedError means the provider is a real interface with no guest creation yet.
type NotImplementedError struct {
	Provider string
	Op       string
}

func (e *NotImplementedError) Error() string {
	return e.Provider + " " + e.Op + " is not implemented"
}

// Unconfigured is the provisioner used when the operator has not chosen one.
type Unconfigured struct{}

func (Unconfigured) Name() string { return "none" }

func (Unconfigured) Provision(context.Context, Spec) (Machine, error) {
	return Machine{}, &NotImplementedError{Provider: "none", Op: "create guest"}
}

func (Unconfigured) Health(context.Context, Runner) (Signal, Signal) {
	return Signal{State: HealthUnknown, Detail: "no provisioner is configured"},
		Signal{State: HealthUnknown, Detail: "host load is not probed"}
}

// Incus records the provider name. Guest creation and probes are not implemented.
type Incus struct{}

func (Incus) Name() string { return "incus" }

func (Incus) Provision(context.Context, Spec) (Machine, error) {
	return Machine{}, &NotImplementedError{Provider: "incus", Op: "create guest"}
}

func (Incus) Health(context.Context, Runner) (Signal, Signal) {
	return Signal{State: HealthUnknown, Detail: "Incus guest probe is not implemented"},
		Signal{State: HealthUnknown, Detail: "Incus host load is not implemented"}
}

// Proxmox can probe a guest that is already listed in ci_runners.json.
// Creating a guest is not implemented.
type Proxmox struct {
	BaseURL string
	Token   string
	Node    string
	Client  *http.Client
}

func (p Proxmox) Name() string { return "proxmox" }

func (p Proxmox) Provision(context.Context, Spec) (Machine, error) {
	return Machine{}, &NotImplementedError{Provider: "proxmox", Op: "create guest"}
}

func (p Proxmox) Health(ctx context.Context, runner Runner) (Signal, Signal) {
	unknown := Signal{State: HealthUnknown, Detail: "Proxmox is not configured"}
	if strings.TrimSpace(p.BaseURL) == "" || strings.TrimSpace(p.Token) == "" || strings.TrimSpace(p.Node) == "" {
		return unknown, Signal{State: HealthUnknown, Detail: "host load is not probed"}
	}
	if runner.VMID <= 0 || (runner.Kind != "qemu" && runner.Kind != "lxc") {
		return Signal{State: HealthUnknown, Detail: "no Proxmox guest is recorded for this runner"},
			Signal{State: HealthUnknown, Detail: "host load is not probed"}
	}
	base := strings.TrimRight(p.BaseURL, "/")
	path := fmt.Sprintf("%s/api2/json/nodes/%s/%s/%d/status/current", base, url.PathEscape(p.Node), runner.Kind, runner.VMID)
	body, code, err := p.call(ctx, http.MethodGet, path, nil)
	if err != nil || code != http.StatusOK {
		return Signal{State: HealthUnknown, Detail: fmt.Sprintf("status probe HTTP %d", code)},
			Signal{State: HealthUnknown}
	}
	var parsed struct {
		Data struct {
			Status string  `json:"status"`
			CPU    float64 `json:"cpu"`
			Mem    int64   `json:"mem"`
			MaxMem int64   `json:"maxmem"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Signal{State: HealthUnknown, Detail: "status probe returned unreadable JSON"},
			Signal{State: HealthUnknown}
	}
	sub := Signal{State: HealthOK, Detail: parsed.Data.Status}
	if parsed.Data.Status != "running" {
		sub.State = HealthCritical
		return sub, Signal{State: HealthUnknown, Detail: "guest is not running"}
	}
	if runner.Kind == "qemu" {
		ping := fmt.Sprintf("%s/api2/json/nodes/%s/qemu/%d/agent/ping", base, url.PathEscape(p.Node), runner.VMID)
		_, pingCode, pingErr := p.call(ctx, http.MethodPost, ping, strings.NewReader(""))
		if pingErr != nil || pingCode != http.StatusOK {
			sub.State = HealthWarn
			sub.Detail = fmt.Sprintf("running, guest agent not answering (HTTP %d)", pingCode)
		} else {
			sub.Detail = "running, guest agent alive"
		}
	}
	cpuPct := parsed.Data.CPU * 100
	memPct := 0.0
	if parsed.Data.MaxMem > 0 {
		memPct = float64(parsed.Data.Mem) / float64(parsed.Data.MaxMem) * 100
	}
	load := Signal{State: HealthOK, Detail: fmt.Sprintf("cpu %.0f%%, mem %.0f%%", cpuPct, memPct)}
	if cpuPct >= 85 || memPct >= 85 {
		load.State = HealthWarn
	}
	return sub, load
}

func (p Proxmox) call(ctx context.Context, method, rawURL string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "PVEAPIToken="+p.Token)
	req.Header.Set("Accept", "application/json")
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return data, resp.StatusCode, err
}

func outcomeFromProvision(p Provisioner, ctx context.Context, spec Spec) ProvisionOutcome {
	machine, err := p.Provision(ctx, spec)
	if err != nil {
		return ProvisionOutcome{Implemented: false, Provider: p.Name(), Message: err.Error()}
	}
	return ProvisionOutcome{Implemented: true, Provider: p.Name(), Message: "guest created", Machine: &machine}
}

func skippedProvision(p Provisioner, message string) ProvisionOutcome {
	return ProvisionOutcome{Implemented: false, Skipped: true, Provider: p.Name(), Message: message}
}
