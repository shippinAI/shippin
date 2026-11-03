package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	hostreg "warden/internal/host"
)

type InfrastructureGraph struct {
	GeneratedAt time.Time                  `json:"generated_at"`
	Providers   []InfrastructureProvider   `json:"providers"`
	Summary     InfrastructureGraphSummary `json:"summary"`
}

type InfrastructureGraphSummary struct {
	Providers int `json:"providers"`
	Hosts     int `json:"hosts"`
	Customers int `json:"customers"`
	Resources int `json:"resources"`
}

type InfrastructureProvider struct {
	ID        string               `json:"id"`
	Label     string               `json:"label"`
	Kind      string               `json:"kind"`
	Status    string               `json:"status"`
	Addresses []string             `json:"addresses,omitempty"`
	Hosts     []InfrastructureHost `json:"hosts"`
}

type InfrastructureHost struct {
	ID              string                   `json:"id"`
	Name            string                   `json:"name"`
	Kind            string                   `json:"kind"`
	WardenHost      bool                     `json:"warden_host"`
	Addresses       []string                 `json:"addresses,omitempty"`
	Tags            []string                 `json:"tags,omitempty"`
	SystemResources []InfrastructureResource `json:"system_resources,omitempty"`
	Customers       []InfrastructureCustomer `json:"customers,omitempty"`
}

type InfrastructureCustomer struct {
	ID        string                   `json:"id"`
	Label     string                   `json:"label"`
	VLAN      string                   `json:"vlan,omitempty"`
	Tags      []string                 `json:"tags,omitempty"`
	Resources []InfrastructureResource `json:"resources,omitempty"`
}

type InfrastructureResource struct {
	ID      string   `json:"id"`
	Type    string   `json:"type"` // lxc | vm | service
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Owner   string   `json:"owner"` // system | <customer>
	VMID    int      `json:"vmid,omitempty"`
	Node    string   `json:"node,omitempty"`
	VLAN    string   `json:"vlan,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Address string   `json:"address,omitempty"`
}

var (
	infraGraphMu     sync.RWMutex
	infraGraphCache  InfrastructureGraph
	infraGraphCached time.Time
	infraGraphTTL    = 15 * time.Second
)

func getInfrastructureGraph(stats SystemStats) InfrastructureGraph {
	infraGraphMu.RLock()
	if !infraGraphCached.IsZero() && time.Since(infraGraphCached) < infraGraphTTL {
		cached := infraGraphCache
		infraGraphMu.RUnlock()
		return cached
	}
	infraGraphMu.RUnlock()

	graph := assembleInfrastructureGraph(stats)
	infraGraphMu.Lock()
	infraGraphCache = graph
	infraGraphCached = time.Now()
	infraGraphMu.Unlock()
	return graph
}

func assembleInfrastructureGraph(stats SystemStats) InfrastructureGraph {
	graph := InfrastructureGraph{GeneratedAt: time.Now().UTC()}
	providers := map[string]*InfrastructureProvider{}

	addProvider := func(id, label, kind, status string) *InfrastructureProvider {
		if p, ok := providers[id]; ok {
			return p
		}
		p := &InfrastructureProvider{ID: id, Label: label, Kind: kind, Status: status}
		providers[id] = p
		return p
	}

	addHost := func(p *InfrastructureProvider, h InfrastructureHost) *InfrastructureHost {
		for i := range p.Hosts {
			if p.Hosts[i].ID == h.ID {
				if h.WardenHost {
					p.Hosts[i].WardenHost = true
				}
				if len(h.Addresses) > 0 {
					p.Hosts[i].Addresses = uniqueAppend(p.Hosts[i].Addresses, h.Addresses...)
				}
				p.Hosts[i].Tags = uniqueAppend(p.Hosts[i].Tags, h.Tags...)
				return &p.Hosts[i]
			}
		}
		p.Hosts = append(p.Hosts, h)
		return &p.Hosts[len(p.Hosts)-1]
	}

	prox := fetchProxmoxInventory()
	if prox.Configured {
		status := "unknown"
		if prox.Available {
			status = "ok"
		} else if prox.Error != "" {
			status = "degraded"
		}
		p := addProvider("proxmox-x", "Proxmox X", "proxmox", status)
		p.Addresses = uniqueAppend(p.Addresses, fmt.Sprintf("%s:%s", prox.Host, "8006"))
		h := addHost(p, InfrastructureHost{
			ID:         "host:" + prox.Node,
			Name:       prox.Node,
			Kind:       "proxmox-node",
			WardenHost: strings.EqualFold(stats.Hostname, prox.Node),
			Tags:       []string{"provider:proxmox-x", "layer-host"},
		})
		resourceMap := map[string]*InfrastructureCustomer{}
		for _, vm := range prox.VMs {
			tags := parseDelimitedTags(vm.Tags)
			owner, vlan := ownerAndVLANFromTags(tags)
			res := InfrastructureResource{
				ID:     fmt.Sprintf("%s:%d", vm.Type, vm.VMID),
				Type:   vmType(vm.Type),
				Name:   vm.Name,
				Status: vm.Status,
				Owner:  owner,
				VMID:   vm.VMID,
				Node:   vm.Node,
				VLAN:   vlan,
				Tags:   tags,
			}
			if owner == "system" {
				h.SystemResources = append(h.SystemResources, res)
				continue
			}
			c, ok := resourceMap[owner]
			if !ok {
				h.Customers = append(h.Customers, InfrastructureCustomer{
					ID:    owner,
					Label: owner,
					VLAN:  vlan,
					Tags:  []string{"layer-customer"},
				})
				c = &h.Customers[len(h.Customers)-1]
				resourceMap[owner] = c
			}
			if c.VLAN == "" {
				c.VLAN = vlan
			}
			c.Resources = append(c.Resources, res)
		}
	}

	root := discoverHostRegistryRoot()
	if root != "" {
		reg := hostreg.NewRegistry(root)
		if err := reg.Load(); err == nil {
			for _, hh := range reg.All() {
				pid := providerIDForKind(hh)
				plabel := providerLabel(pid)
				p := addProvider(pid, plabel, strings.TrimSuffix(pid, "-x"), "ok")
				addresses := make([]string, 0, len(hh.Addresses))
				for _, a := range hh.Addresses {
					addresses = append(addresses, fmt.Sprintf("%s:%d", a.Host, a.Port))
				}
				hostNode := addHost(p, InfrastructureHost{
					ID:         "host:" + hh.Name,
					Name:       hh.Name,
					Kind:       string(hh.Kind),
					WardenHost: strings.EqualFold(hh.Name, stats.Hostname),
					Addresses:  addresses,
					Tags:       append([]string{"layer-host", "provider:" + pid}, hh.Tags...),
				})
				if hh.OwnerClient != "" {
					customer := InfrastructureCustomer{
						ID:    hh.OwnerClient,
						Label: hh.OwnerClient,
						VLAN:  hh.VLAN,
						Tags:  []string{"layer-customer"},
					}
					found := false
					for i := range hostNode.Customers {
						if hostNode.Customers[i].ID == customer.ID {
							found = true
							break
						}
					}
					if !found {
						hostNode.Customers = append(hostNode.Customers, customer)
					}
				}
			}
		}
	}

	for _, ps := range listPlatformSystems() {
		if strings.EqualFold(ps.ID, "warden") {
			continue
		}
		pid := providerIDFromPlatform(ps)
		p := addProvider(pid, providerLabel(pid), strings.TrimSuffix(pid, "-x"), "ok")
		host := addHost(p, InfrastructureHost{
			ID:         "platform:" + ps.ID,
			Name:       ps.Name,
			Kind:       "platform-service",
			WardenHost: false,
			Tags:       []string{"layer-host", "platform", "provider:" + pid},
		})
		res := InfrastructureResource{
			ID:      "svc:" + ps.ID,
			Type:    "service",
			Name:    ps.Name,
			Status:  "unknown",
			Owner:   scopeToOwner(ps.Scope),
			VLAN:    vlanFromTags(ps.Tags),
			Tags:    append([]string{}, ps.Tags...),
			Address: ps.PublicURL,
		}
		if ps.Scope == "customer" {
			host.Customers = append(host.Customers, InfrastructureCustomer{
				ID:        res.Owner,
				Label:     res.Owner,
				VLAN:      res.VLAN,
				Tags:      []string{"layer-customer", "platform"},
				Resources: []InfrastructureResource{res},
			})
		} else {
			host.SystemResources = append(host.SystemResources, res)
		}
	}

	for _, p := range providers {
		sort.Slice(p.Hosts, func(i, j int) bool { return p.Hosts[i].Name < p.Hosts[j].Name })
		for i := range p.Hosts {
			sort.Slice(p.Hosts[i].Customers, func(a, b int) bool {
				return p.Hosts[i].Customers[a].Label < p.Hosts[i].Customers[b].Label
			})
		}
		graph.Providers = append(graph.Providers, *p)
	}
	sort.Slice(graph.Providers, func(i, j int) bool { return graph.Providers[i].Label < graph.Providers[j].Label })
	graph.Summary = summarizeInfrastructureGraph(graph)
	return graph
}

func summarizeInfrastructureGraph(g InfrastructureGraph) InfrastructureGraphSummary {
	s := InfrastructureGraphSummary{Providers: len(g.Providers)}
	for _, p := range g.Providers {
		s.Hosts += len(p.Hosts)
		for _, h := range p.Hosts {
			s.Resources += len(h.SystemResources)
			s.Customers += len(h.Customers)
			for _, c := range h.Customers {
				s.Resources += len(c.Resources)
			}
		}
	}
	return s
}

func graphToDashboardNodes(graph InfrastructureGraph, stats SystemStats) []HostNode {
	nodes := make([]HostNode, 0, graph.Summary.Providers+graph.Summary.Hosts+graph.Summary.Customers+graph.Summary.Resources)
	for _, p := range graph.Providers {
		state := HealthUnknown
		if p.Status == "ok" {
			state = HealthOK
		} else if p.Status == "degraded" {
			state = HealthWarn
		}
		nodes = append(nodes, HostNode{
			ID:      "provider:" + p.ID,
			Name:    p.Label,
			Kind:    "provider-x",
			Scope:   "internal",
			Status:  state,
			Address: strings.Join(p.Addresses, ", "),
			Tags:    []string{"layer-provider", "provider:" + p.ID},
		})
		for _, h := range p.Hosts {
			nodes = append(nodes, HostNode{
				ID:      h.ID,
				Name:    h.Name,
				Kind:    h.Kind,
				Scope:   "internal",
				Status:  HealthOK,
				Address: strings.Join(h.Addresses, ", "),
				Tags:    uniqueAppend(h.Tags, "layer-host"),
				Connections: []HostConnection{
					{Label: "Provider", Target: p.Label, State: state},
				},
			})
			for _, c := range h.Customers {
				cstate := HealthUnknown
				if len(c.Resources) > 0 {
					cstate = HealthOK
				}
				nodes = append(nodes, HostNode{
					ID:      "customer:" + h.ID + ":" + c.ID,
					Name:    c.Label,
					Kind:    "clyffe-customer",
					Scope:   "customer",
					Status:  cstate,
					Address: c.VLAN,
					Tags:    uniqueAppend(c.Tags, "layer-customer", "customer:"+c.ID),
					Connections: []HostConnection{
						{Label: "Host", Target: h.Name, State: HealthOK},
					},
				})
				for _, r := range c.Resources {
					nodes = append(nodes, HostNode{
						ID:      "resource:" + r.ID,
						Name:    r.Name,
						Kind:    "resource-" + r.Type,
						Scope:   "customer",
						Status:  statusToHealth(r.Status),
						Address: r.Address,
						Tags:    uniqueAppend(r.Tags, "layer-resource", "customer:"+c.ID, "vlan:"+r.VLAN),
						Connections: []HostConnection{
							{Label: "Tenant", Target: c.Label, State: HealthOK},
							{Label: "Runtime", Target: fmt.Sprintf("%s/%d", r.Type, r.VMID), State: statusToHealth(r.Status)},
						},
					})
				}
			}
			for _, r := range h.SystemResources {
				nodes = append(nodes, HostNode{
					ID:      "resource:" + h.ID + ":" + r.ID,
					Name:    r.Name,
					Kind:    "resource-" + r.Type,
					Scope:   "internal",
					Status:  statusToHealth(r.Status),
					Address: r.Address,
					Tags:    uniqueAppend(r.Tags, "layer-resource", "system-owned"),
					Connections: []HostConnection{
						{Label: "Host", Target: h.Name, State: HealthOK},
						{Label: "Runtime", Target: fmt.Sprintf("%s/%d", r.Type, r.VMID), State: statusToHealth(r.Status)},
					},
				})
			}
		}
	}
	if len(nodes) == 0 {
		nodes = append(nodes, HostNode{
			ID:      "local",
			Name:    stats.Hostname,
			Kind:    "warden-host",
			Scope:   "internal",
			Status:  aggregateHealth(stats),
			Address: "local",
			Tags:    []string{"layer-host", "fallback"},
		})
	}
	return nodes
}

func discoverHostRegistryRoot() string {
	if d := strings.TrimSpace(getEnvOr("", "WARDEN_HOSTS_DIR")); d != "" {
		return d
	}
	candidates := []string{
		"/etc/warden",
		filepath.Join(".", "internal", "host", "testdata"),
	}
	for _, c := range candidates {
		if fileExists(filepath.Join(c, "hosts.yaml")) {
			return c
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func providerIDForKind(h *hostreg.Host) string {
	if h.Provider != "" {
		return h.Provider
	}
	switch h.Kind {
	case hostreg.KindProxmox, hostreg.KindWardenclyffe:
		return "proxmox-x"
	default:
		return "bare-x"
	}
}

func providerIDFromPlatform(ps PlatformSystem) string {
	s := strings.ToLower(ps.Category + " " + strings.Join(ps.Tags, " "))
	switch {
	case strings.Contains(s, "proxmox"):
		return "proxmox-x"
	case strings.Contains(s, "solus"):
		return "solus-x"
	default:
		return "service-x"
	}
}

func providerLabel(id string) string {
	switch id {
	case "proxmox-x":
		return "Proxmox X"
	case "solus-x":
		return "Solus X"
	case "service-x":
		return "Service Edge X"
	default:
		return strings.TrimSuffix(strings.Title(strings.ReplaceAll(id, "-", " ")), " X") + " X"
	}
}

func parseDelimitedTags(tags string) []string {
	split := strings.FieldsFunc(tags, func(r rune) bool {
		return r == ';' || r == ',' || r == ' '
	})
	out := make([]string, 0, len(split))
	for _, t := range split {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func ownerAndVLANFromTags(tags []string) (string, string) {
	owner := "system"
	vlan := ""
	for _, t := range tags {
		lt := strings.ToLower(t)
		switch {
		case strings.HasPrefix(lt, "tenant-"):
			owner = strings.TrimPrefix(lt, "tenant-")
		case strings.HasPrefix(lt, "customer-"):
			owner = strings.TrimPrefix(lt, "customer-")
		case strings.HasPrefix(lt, "clyffe:"):
			owner = strings.TrimPrefix(lt, "clyffe:")
		case strings.HasPrefix(lt, "vlan-"):
			vlan = strings.TrimPrefix(lt, "vlan-")
		case strings.HasPrefix(lt, "vlan:"):
			vlan = strings.TrimPrefix(lt, "vlan:")
		}
	}
	return owner, vlan
}

func statusToHealth(status string) HealthState {
	switch strings.ToLower(status) {
	case "running", "up", "healthy", "ok":
		return HealthOK
	case "stopped", "down", "failed":
		return HealthCritical
	default:
		return HealthUnknown
	}
}

func vmType(raw string) string {
	if strings.EqualFold(raw, "qemu") {
		return "vm"
	}
	return "lxc"
}

func scopeToOwner(scope string) string {
	if scope == "customer" {
		return "customer-shared"
	}
	return "system"
}

func vlanFromTags(tags []string) string {
	for _, t := range tags {
		if strings.HasPrefix(strings.ToLower(t), "vlan-") {
			return strings.TrimPrefix(strings.ToLower(t), "vlan-")
		}
	}
	return ""
}

func uniqueAppend(base []string, add ...string) []string {
	seen := map[string]bool{}
	for _, v := range base {
		seen[v] = true
	}
	for _, v := range add {
		if v == "" || seen[v] {
			continue
		}
		base = append(base, v)
		seen[v] = true
	}
	return base
}

func getEnvOr(def string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return def
}

func parseIntDefault(raw string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return def
	}
	return v
}
