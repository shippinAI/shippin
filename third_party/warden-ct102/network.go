package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

// NetworkInterface extends NetInfo with more details
type NetworkInterface struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	IPv4      string `json:"ipv4"`
	IPv6      string `json:"ipv6"`
	MAC       string `json:"mac"`
	MTU       string `json:"mtu"`
	Speed     string `json:"speed"` // link speed in Mbps
	RxBytes   uint64 `json:"rxBytes"`
	TxBytes   uint64 `json:"txBytes"`
	RxPackets uint64 `json:"rxPackets"`
	TxPackets uint64 `json:"txPackets"`
	RxErrors  uint64 `json:"rxErrors"`
	TxErrors  uint64 `json:"txErrors"`
	Type      string `json:"type"` // physical, virtual, bridge, loopback
}

// ActiveConnection represents a network connection from ss
type ActiveConnection struct {
	Proto     string `json:"proto"`
	State     string `json:"state"`
	LocalAddr string `json:"localAddr"`
	LocalPort int    `json:"localPort"`
	PeerAddr  string `json:"peerAddr"`
	PeerPort  int    `json:"peerPort"`
	Process   string `json:"process"`
	PID       int    `json:"pid"`
}

// RouteEntry represents a routing table entry
type RouteEntry struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Iface       string `json:"iface"`
	Metric      string `json:"metric"`
	Flags       string `json:"flags"`
}

// ConnectionSummary has counts by state
type ConnectionSummary struct {
	Established int `json:"established"`
	TimeWait    int `json:"timeWait"`
	CloseWait   int `json:"closeWait"`
	Listen      int `json:"listen"`
	SynSent     int `json:"synSent"`
	SynRecv     int `json:"synRecv"`
	FinWait     int `json:"finWait"`
	Other       int `json:"other"`
	Total       int `json:"total"`
}

type NetworkPageData struct {
	HeaderData
	Interfaces   []NetworkInterface
	ListenPorts  []ListenPort
	Connections  []ActiveConnection
	ConnSummary  ConnectionSummary
	Routes       []RouteEntry
	DNS          []string
	Hostname     string
	DefaultGW    string
	PublicIP     string
}

func getNetworkInterfaces() []NetworkInterface {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil
	}

	var ifaces []NetworkInterface
	lines := strings.Split(string(data), "\n")

	for _, line := range lines[2:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colonIdx])
		rest := strings.Fields(line[colonIdx+1:])
		if len(rest) < 16 {
			continue
		}

		iface := NetworkInterface{
			Name:      name,
			RxBytes:   parseUint(rest[0]),
			RxPackets: parseUint(rest[1]),
			RxErrors:  parseUint(rest[2]),
			TxBytes:   parseUint(rest[8]),
			TxPackets: parseUint(rest[9]),
			TxErrors:  parseUint(rest[10]),
			State:     getIfState(name),
			IPv4:      getIPv4(name),
			Type:      classifyInterface(name),
		}

		// MAC address
		macData, err := os.ReadFile("/sys/class/net/" + name + "/address")
		if err == nil {
			iface.MAC = strings.TrimSpace(string(macData))
		}

		// MTU
		mtuData, err := os.ReadFile("/sys/class/net/" + name + "/mtu")
		if err == nil {
			iface.MTU = strings.TrimSpace(string(mtuData))
		}

		// Link speed
		speedData, err := os.ReadFile("/sys/class/net/" + name + "/speed")
		if err == nil {
			s := strings.TrimSpace(string(speedData))
			if s != "" && s != "-1" {
				iface.Speed = s + " Mbps"
			}
		}

		// IPv6
		iface.IPv6 = getIPv6(name)

		ifaces = append(ifaces, iface)
	}

	// Sort: physical first, then by name
	sort.Slice(ifaces, func(i, j int) bool {
		if ifaces[i].Type != ifaces[j].Type {
			order := map[string]int{"physical": 0, "loopback": 1, "bridge": 2, "virtual": 3}
			return order[ifaces[i].Type] < order[ifaces[j].Type]
		}
		return ifaces[i].Name < ifaces[j].Name
	})

	return ifaces
}

func classifyInterface(name string) string {
	if name == "lo" {
		return "loopback"
	}
	if strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "docker") {
		return "bridge"
	}
	if strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "virbr") ||
		strings.HasPrefix(name, "tun") || strings.HasPrefix(name, "tap") ||
		strings.HasPrefix(name, "flannel") || strings.HasPrefix(name, "cni") {
		return "virtual"
	}
	// Check if it's a virtual device
	path := "/sys/class/net/" + name + "/device"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// No physical device backing — but eth/ens/enp are often VMs
		if strings.HasPrefix(name, "eth") || strings.HasPrefix(name, "ens") ||
			strings.HasPrefix(name, "enp") || strings.HasPrefix(name, "wl") {
			return "physical"
		}
		return "virtual"
	}
	return "physical"
}

func getIPv6(name string) string {
	data, err := os.ReadFile("/proc/net/if_inet6")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		if fields[5] == name {
			addr := fields[0]
			// Format: insert colons every 4 chars
			var parts []string
			for i := 0; i < len(addr); i += 4 {
				end := i + 4
				if end > len(addr) {
					end = len(addr)
				}
				parts = append(parts, addr[i:end])
			}
			formatted := strings.Join(parts, ":")
			// Skip link-local if we find a global one
			if !strings.HasPrefix(formatted, "fe80") {
				return formatted
			}
		}
	}
	return ""
}

func getActiveConnections() ([]ActiveConnection, ConnectionSummary) {
	out, err := execCommand("ss", "-tunap")
	if err != nil {
		return nil, ConnectionSummary{}
	}

	var conns []ActiveConnection
	var summary ConnectionSummary

	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		proto := fields[0]
		state := fields[1]

		// For UDP, state field is different
		localIdx := 4
		peerIdx := 5
		if proto == "udp" || proto == "udp6" {
			state = fields[1]
			if state == "UNCONN" || state == "ESTAB" {
				localIdx = 4
				peerIdx = 5
			}
		}

		if localIdx >= len(fields) || peerIdx >= len(fields) {
			continue
		}

		localFull := fields[localIdx]
		peerFull := fields[peerIdx]

		localAddr, localPort := splitAddrPort(localFull)
		peerAddr, peerPort := splitAddrPort(peerFull)

		// Parse process
		procName := ""
		pid := 0
		for _, f := range fields[6:] {
			if strings.HasPrefix(f, "users:") {
				if start := strings.Index(f, "((\""); start >= 0 {
					end := strings.Index(f[start+3:], "\"")
					if end >= 0 {
						procName = f[start+3 : start+3+end]
					}
				}
				if pidIdx := strings.Index(f, "pid="); pidIdx >= 0 {
					rest := f[pidIdx+4:]
					if comma := strings.IndexAny(rest, ",)"); comma >= 0 {
						pid, _ = strconv.Atoi(rest[:comma])
					}
				}
			}
		}

		conn := ActiveConnection{
			Proto:     proto,
			State:     state,
			LocalAddr: localAddr,
			LocalPort: localPort,
			PeerAddr:  peerAddr,
			PeerPort:  peerPort,
			Process:   procName,
			PID:       pid,
		}
		conns = append(conns, conn)

		// Count states
		summary.Total++
		switch state {
		case "ESTAB":
			summary.Established++
		case "TIME-WAIT":
			summary.TimeWait++
		case "CLOSE-WAIT":
			summary.CloseWait++
		case "LISTEN":
			summary.Listen++
		case "SYN-SENT":
			summary.SynSent++
		case "SYN-RECV":
			summary.SynRecv++
		case "FIN-WAIT-1", "FIN-WAIT-2":
			summary.FinWait++
		default:
			summary.Other++
		}
	}

	return conns, summary
}

func splitAddrPort(s string) (string, int) {
	lastColon := strings.LastIndex(s, ":")
	if lastColon < 0 {
		return s, 0
	}
	addr := s[:lastColon]
	port, _ := strconv.Atoi(s[lastColon+1:])
	return addr, port
}

func getRoutes() []RouteEntry {
	out, err := execCommand("ip", "route")
	if err != nil {
		return nil
	}

	var routes []RouteEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		route := RouteEntry{
			Destination: fields[0],
		}

		for i := 1; i < len(fields)-1; i++ {
			switch fields[i] {
			case "via":
				route.Gateway = fields[i+1]
			case "dev":
				route.Iface = fields[i+1]
			case "metric":
				route.Metric = fields[i+1]
			}
		}

		// Collect flags
		if strings.Contains(line, "proto kernel") {
			route.Flags = "kernel"
		} else if strings.Contains(line, "proto static") {
			route.Flags = "static"
		} else if strings.Contains(line, "proto dhcp") {
			route.Flags = "dhcp"
		}

		routes = append(routes, route)
	}

	return routes
}

func getDNSServers() []string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}

	var servers []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				servers = append(servers, fields[1])
			}
		}
	}
	return servers
}

func getDefaultGateway() string {
	out, err := execCommand("ip", "route", "show", "default")
	if err != nil {
		return ""
	}
	fields := strings.Fields(strings.TrimSpace(out))
	for i, f := range fields {
		if f == "via" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

func getPublicIP() string {
	// Try reading from a cached result or quick command
	out, err := execCommand("curl", "-s", "--connect-timeout", "2", "--max-time", "3", "https://ifconfig.me/ip")
	if err != nil || strings.TrimSpace(out) == "" {
		out, err = execCommand("curl", "-s", "--connect-timeout", "2", "--max-time", "3", "https://api.ipify.org")
		if err != nil {
			return "unavailable"
		}
	}
	return strings.TrimSpace(out)
}

func handleNetwork(w http.ResponseWriter, r *http.Request) {
	hd := buildHeaderData(r)
	hd.PageTitle = "Network"

	conns, summary := getActiveConnections()
	hostname, _ := os.Hostname()

	// Only show established connections by default (not LISTEN, those are in ports)
	var activeConns []ActiveConnection
	for _, c := range conns {
		if c.State != "LISTEN" {
			activeConns = append(activeConns, c)
		}
	}

	// Limit to 200 connections for display
	if len(activeConns) > 200 {
		activeConns = activeConns[:200]
	}

	data := NetworkPageData{
		HeaderData:  hd,
		Interfaces:  getNetworkInterfaces(),
		ListenPorts: getListeningPorts(),
		Connections: activeConns,
		ConnSummary: summary,
		Routes:      getRoutes(),
		DNS:         getDNSServers(),
		Hostname:    hostname,
		DefaultGW:   getDefaultGateway(),
		PublicIP:    getPublicIP(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "network.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPINetwork(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")

	switch action {
	case "connections":
		conns, summary := getActiveConnections()
		var active []ActiveConnection
		for _, c := range conns {
			if c.State != "LISTEN" {
				active = append(active, c)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"connections": active,
			"summary":    summary,
		})

	case "interfaces":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"interfaces": getNetworkInterfaces(),
		})

	default:
		hostname, _ := os.Hostname()
		conns, summary := getActiveConnections()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"interfaces":  getNetworkInterfaces(),
			"listenPorts": getListeningPorts(),
			"connections": conns,
			"connSummary": summary,
			"routes":      getRoutes(),
			"dns":         getDNSServers(),
			"hostname":    hostname,
			"defaultGW":   getDefaultGateway(),
		})
	}
}
