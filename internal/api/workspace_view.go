package api

import (
	"fmt"
	"strings"
	"time"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/hostd"
)

type connectInfo struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	User      string `json:"user"`
	SSH       string `json:"ssh"`
	SSHConfig string `json:"ssh_config"`
	VSCode    string `json:"vscode"`
}

type workspaceView struct {
	db.Workspace
	Observation   string       `json:"observation"`
	ObservedAt    *time.Time   `json:"observed_at,omitempty"`
	RecordedState string       `json:"recorded_state"`
	Instance      string       `json:"instance"`
	MemoryBytes   int64        `json:"memory_bytes"`
	KeyCount      int          `json:"ssh_key_count"`
	Connect       *connectInfo `json:"connect,omitempty"`
	URL           string       `json:"url"`
	PublicIP      string       `json:"public_ip"`
	Region        string       `json:"region"`
	VSCodeURL     string       `json:"vscode_url,omitempty"`
	IDEPort       int          `json:"ide_port"`
	PreviewURLs   []previewURL `json:"preview_urls"`
}

type previewURL struct {
	db.Preview
	URL string `json:"url"`
}

// view merges the DB row with live Incus state and connect details.
func (a *API) view(t *tenantView, w db.Workspace, live map[string]hostclient.Instance) workspaceView {
	v := workspaceView{Workspace: w, Instance: "w-" + w.ID, KeyCount: len(w.SSHKeys),
		RecordedState: w.State, Observation: "unavailable", URL: "https://" + w.Host, PublicIP: t.SSHIP, Region: a.cfg.Incus.HostLabel, IDEPort: a.cfg.Workspaces.IDEPort}
	if v.Domains == nil {
		v.Domains = []string{}
	}
	if v.Firewall == nil {
		v.Firewall = []db.Rule{}
	}
	v.PreviewURLs = []previewURL{}
	for _, p := range w.Previews {
		v.PreviewURLs = append(v.PreviewURLs, previewURL{p, "https://" + hostd.PreviewHost(w.Host, p.Port)})
	}
	if live != nil {
		v.Observation = "missing"
	}
	if v.State == "running" || v.State == "stopped" {
		v.State = "unknown"
	}
	if i, ok := live[v.Instance]; ok {
		now := time.Now().UTC()
		v.ObservedAt = &now
		v.Observation = "observed"
		if i.State != nil {
			v.MemoryBytes = i.State.Memory.Usage
		}
		if w.State == "running" || w.State == "stopped" {
			switch state := strings.ToLower(i.Status); state {
			case "running", "stopped":
				v.State = state
			}
		}
		if ip := i.IPv4(); ip != "" {
			v.IPv4 = ip
		}
	}
	if t.SSHIP != "" {
		u, alias := a.cfg.Workspaces.User, fmt.Sprintf("%s-%s", strings.ToLower(strings.ReplaceAll(t.Name, " ", "-")), w.Name)
		v.Connect = &connectInfo{
			Host: t.SSHIP, Port: w.SSHPort, User: u,
			SSH:       fmt.Sprintf("ssh -p %d %s@%s", w.SSHPort, u, t.SSHIP),
			SSHConfig: fmt.Sprintf("Host %s\n  HostName %s\n  Port %d\n  User %s", alias, t.SSHIP, w.SSHPort, u),
			VSCode:    fmt.Sprintf("code --remote ssh-remote+%s %s", alias, a.cfg.Workspaces.CodeDir),
		}
		v.VSCodeURL = fmt.Sprintf("vscode://vscode-remote/ssh-remote+%s@%s:%d%s", u, t.SSHIP, w.SSHPort, a.cfg.Workspaces.CodeDir)
	}
	return v
}
