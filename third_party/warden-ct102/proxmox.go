package main

// Proxmox VE connector.
//
// Talks to a remote Proxmox host via API token (PVEAPIToken header).
// Token + host config read from /etc/warden/warden.env or environment.
//
// Endpoints:
//   GET  /proxmox          — page (list VMs/CTs across the configured node)
//   GET  /api/proxmox      — JSON: full inventory
//   POST /api/proxmox      — actions: vm-start, vm-stop, vm-shutdown, vm-reboot,
//                            vm-snapshot, ct-start, ct-stop, ct-shutdown, ct-reboot
//
// Configuration (any of):
//   - env vars: PROXMOX_HOST, PROXMOX_PORT, PROXMOX_NODE, PROXMOX_TOKEN_ID,
//               PROXMOX_TOKEN_SECRET, PROXMOX_VERIFY_TLS
//   - /etc/warden/warden.env (loaded by the systemd unit)

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --- types ---------------------------------------------------------------

type ProxmoxConfig struct {
	Host        string
	Port        string
	Node        string
	TokenID     string
	TokenSecret string
	VerifyTLS   bool
}

type ProxmoxResource struct {
	VMID     int     `json:"vmid"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`   // qemu | lxc
	Status   string  `json:"status"` // running | stopped | template
	Template int     `json:"template"`
	Node     string  `json:"node"`
	Pool     string  `json:"pool,omitempty"`
	Tags     string  `json:"tags,omitempty"`
	Uptime   int64   `json:"uptime"`
	CPU      float64 `json:"cpu"`
	MaxCPU   int     `json:"maxcpu"`
	Mem      int64   `json:"mem"`
	MaxMem   int64   `json:"maxmem"`
	Disk     int64   `json:"disk"`
	MaxDisk  int64   `json:"maxdisk"`
	NetIn    int64   `json:"netin"`
	NetOut   int64   `json:"netout"`
}

type ProxmoxStorage struct {
	Storage string  `json:"storage"`
	Type    string  `json:"type"`
	Total   int64   `json:"total"`
	Used    int64   `json:"used"`
	Avail   int64   `json:"avail"`
	UsedPct float64 `json:"used_pct"`
	Active  int     `json:"active"`
	Enabled int     `json:"enabled"`
	Shared  int     `json:"shared"`
	Content string  `json:"content,omitempty"`
}

type ProxmoxNodeStatus struct {
	Node       string   `json:"node"`
	Status     string   `json:"status"`
	PVEVersion string   `json:"pveversion"`
	Uptime     int64    `json:"uptime"`
	CPUUsage   float64  `json:"cpu"`
	MemUsed    int64    `json:"mem_used"`
	MemTotal   int64    `json:"mem_total"`
	LoadAvg    []string `json:"loadavg,omitempty"`
}

type ProxmoxInventory struct {
	Available  bool              `json:"available"`
	Configured bool              `json:"configured"`
	Error      string            `json:"error,omitempty"`
	Host       string            `json:"host,omitempty"`
	Node       string            `json:"node,omitempty"`
	Status     ProxmoxNodeStatus `json:"status"`
	VMs        []ProxmoxResource `json:"vms"`
	Storage    []ProxmoxStorage  `json:"storage"`
}

type ProxmoxPageData struct {
	HeaderData
	Inventory ProxmoxInventory
}

// --- config + http client ------------------------------------------------

var (
	proxmoxConfigOnce sync.Once
	proxmoxConfig     ProxmoxConfig
	proxmoxConfigErr  error

	proxmoxHTTPOnce sync.Once
	proxmoxHTTP     *http.Client
)

func getProxmoxConfig() (ProxmoxConfig, error) {
	proxmoxConfigOnce.Do(func() {
		c := ProxmoxConfig{
			Host:        os.Getenv("PROXMOX_HOST"),
			Port:        os.Getenv("PROXMOX_PORT"),
			Node:        os.Getenv("PROXMOX_NODE"),
			TokenID:     os.Getenv("PROXMOX_TOKEN_ID"),
			TokenSecret: os.Getenv("PROXMOX_TOKEN_SECRET"),
		}
		if c.Port == "" {
			c.Port = "8006"
		}
		c.VerifyTLS = !strings.EqualFold(os.Getenv("PROXMOX_VERIFY_TLS"), "false")

		if c.Host == "" || c.TokenID == "" || c.TokenSecret == "" {
			proxmoxConfigErr = fmt.Errorf("not configured: set PROXMOX_HOST, PROXMOX_TOKEN_ID, PROXMOX_TOKEN_SECRET in /etc/warden/warden.env")
			return
		}
		proxmoxConfig = c
	})
	return proxmoxConfig, proxmoxConfigErr
}

func getProxmoxHTTP() *http.Client {
	proxmoxHTTPOnce.Do(func() {
		cfg, _ := getProxmoxConfig()
		proxmoxHTTP = &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: !cfg.VerifyTLS,
				},
			},
		}
	})
	return proxmoxHTTP
}

func proxmoxAPI(method, path string, body url.Values) ([]byte, int, error) {
	cfg, err := getProxmoxConfig()
	if err != nil {
		return nil, 0, err
	}

	urlStr := fmt.Sprintf("https://%s:%s/api2/json%s", cfg.Host, cfg.Port, path)

	var req *http.Request
	if body != nil {
		req, err = http.NewRequest(method, urlStr, strings.NewReader(body.Encode()))
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequest(method, urlStr, nil)
		if err != nil {
			return nil, 0, err
		}
	}
	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", cfg.TokenID, cfg.TokenSecret))

	resp, err := getProxmoxHTTP().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// --- inventory --------------------------------------------------------

func fetchProxmoxInventory() ProxmoxInventory {
	inv := ProxmoxInventory{}

	cfg, err := getProxmoxConfig()
	if err != nil {
		inv.Error = err.Error()
		return inv
	}
	inv.Configured = true
	inv.Host = cfg.Host
	inv.Node = cfg.Node

	// version probe — also doubles as auth check
	verData, code, err := proxmoxAPI("GET", "/version", nil)
	if err != nil || code != 200 {
		if err != nil {
			inv.Error = err.Error()
		} else {
			inv.Error = fmt.Sprintf("version probe HTTP %d: %s", code, truncateString(string(verData), 200))
		}
		return inv
	}
	inv.Available = true

	// version goes into status
	var verResp struct {
		Data struct {
			Version string `json:"version"`
			Release string `json:"release"`
		} `json:"data"`
	}
	json.Unmarshal(verData, &verResp)
	inv.Status.PVEVersion = verResp.Data.Version

	// VMs/CTs via cluster/resources
	vmData, code, err := proxmoxAPI("GET", "/cluster/resources?type=vm", nil)
	if err == nil && code == 200 {
		var r struct {
			Data []ProxmoxResource `json:"data"`
		}
		if err := json.Unmarshal(vmData, &r); err == nil {
			inv.VMs = r.Data
		}
	}

	// node-level status
	nodeStatusPath := fmt.Sprintf("/nodes/%s/status", cfg.Node)
	if data, code, err := proxmoxAPI("GET", nodeStatusPath, nil); err == nil && code == 200 {
		var r struct {
			Data struct {
				Uptime int64   `json:"uptime"`
				CPU    float64 `json:"cpu"`
				Memory struct {
					Used  int64 `json:"used"`
					Total int64 `json:"total"`
				} `json:"memory"`
				LoadAvg []string `json:"loadavg"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &r); err == nil {
			inv.Status.Node = cfg.Node
			inv.Status.Status = "online"
			inv.Status.Uptime = r.Data.Uptime
			inv.Status.CPUUsage = r.Data.CPU * 100
			inv.Status.MemUsed = r.Data.Memory.Used
			inv.Status.MemTotal = r.Data.Memory.Total
			inv.Status.LoadAvg = r.Data.LoadAvg
		}
	}

	// storage
	storagePath := fmt.Sprintf("/nodes/%s/storage", cfg.Node)
	if data, code, err := proxmoxAPI("GET", storagePath, nil); err == nil && code == 200 {
		var r struct {
			Data []ProxmoxStorage `json:"data"`
		}
		if err := json.Unmarshal(data, &r); err == nil {
			for i := range r.Data {
				if r.Data[i].Total > 0 {
					r.Data[i].UsedPct = float64(r.Data[i].Used) / float64(r.Data[i].Total) * 100
				}
			}
			inv.Storage = r.Data
		}
	}

	return inv
}

// --- handlers --------------------------------------------------------

func handleProxmox(w http.ResponseWriter, r *http.Request) {
	hd := buildHeaderData(r)
	hd.PageTitle = "Proxmox"

	data := ProxmoxPageData{
		HeaderData: hd,
		Inventory:  fetchProxmoxInventory(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "proxmox.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIProxmox(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		inv := fetchProxmoxInventory()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   inv.Available || inv.Configured,
			"inventory": inv,
		})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.ParseForm()
	action := r.FormValue("action")

	switch action {
	case "vm-start", "vm-stop", "vm-shutdown", "vm-reboot",
		"ct-start", "ct-stop", "ct-shutdown", "ct-reboot":
		handleProxmoxLifecycle(w, r, action)
	case "vm-snapshot":
		handleProxmoxSnapshot(w, r)
	default:
		respondJSON(w, false, "Invalid action")
	}
}

func handleProxmoxLifecycle(w http.ResponseWriter, r *http.Request, action string) {
	cfg, err := getProxmoxConfig()
	if err != nil {
		respondJSON(w, false, err.Error())
		return
	}

	vmidStr := r.FormValue("vmid")
	vmid, err := strconv.Atoi(vmidStr)
	if err != nil || vmid < 100 || vmid > 999999 {
		respondJSON(w, false, "Invalid VMID")
		return
	}

	parts := strings.SplitN(action, "-", 2)
	if len(parts) != 2 {
		respondJSON(w, false, "Invalid action")
		return
	}
	kind, verb := parts[0], parts[1] // vm|ct, start|stop|shutdown|reboot

	resource := "qemu"
	if kind == "ct" {
		resource = "lxc"
	}
	path := fmt.Sprintf("/nodes/%s/%s/%d/status/%s", cfg.Node, resource, vmid, verb)

	data, code, err := proxmoxAPI("POST", path, url.Values{})
	if err != nil {
		respondJSON(w, false, "API call failed: "+err.Error())
		return
	}
	if code != 200 {
		respondJSON(w, false, fmt.Sprintf("HTTP %d: %s", code, truncateString(string(data), 200)))
		return
	}

	auditFromRequest(r, fmt.Sprintf("Proxmox %s", action), fmt.Sprintf("vmid=%d", vmid), "proxmox")
	respondJSON(w, true, fmt.Sprintf("%s issued for VMID %d", action, vmid))
}

func handleProxmoxSnapshot(w http.ResponseWriter, r *http.Request) {
	cfg, err := getProxmoxConfig()
	if err != nil {
		respondJSON(w, false, err.Error())
		return
	}

	vmidStr := r.FormValue("vmid")
	vmid, err := strconv.Atoi(vmidStr)
	if err != nil || vmid < 100 || vmid > 999999 {
		respondJSON(w, false, "Invalid VMID")
		return
	}

	snapname := r.FormValue("snapname")
	if snapname == "" {
		snapname = fmt.Sprintf("warden-%s", time.Now().Format("20060102-150405"))
	}
	// PVE snapname rules: [a-z][a-z0-9_-]{2,39}
	if !isValidSnapName(snapname) {
		respondJSON(w, false, "Invalid snapshot name (must start with letter, 2-40 chars, a-z 0-9 _ -)")
		return
	}

	body := url.Values{}
	body.Set("snapname", snapname)
	body.Set("description", "snapshot from warden by "+getSessionUserOrAnon(r))

	path := fmt.Sprintf("/nodes/%s/qemu/%d/snapshot", cfg.Node, vmid)
	data, code, err := proxmoxAPI("POST", path, body)
	if err != nil {
		respondJSON(w, false, "API call failed: "+err.Error())
		return
	}
	if code != 200 {
		respondJSON(w, false, fmt.Sprintf("HTTP %d: %s", code, truncateString(string(data), 200)))
		return
	}

	auditFromRequest(r, "Proxmox snapshot", fmt.Sprintf("vmid=%d snapname=%s", vmid, snapname), "proxmox")
	respondJSON(w, true, fmt.Sprintf("Snapshot '%s' created on VMID %d", snapname, vmid))
}

// --- helpers --------------------------------------------------------

func isValidSnapName(s string) bool {
	if len(s) < 2 || len(s) > 40 {
		return false
	}
	if !(s[0] >= 'a' && s[0] <= 'z') {
		return false
	}
	for _, c := range s[1:] {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func getSessionUserOrAnon(r *http.Request) string {
	if u, ok := getSession(r); ok {
		return u
	}
	return "anonymous"
}
