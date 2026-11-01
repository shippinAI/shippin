package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Endpoint struct {
	ID        string    `json:"id"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Label     string    `json:"label"`
	Interval  int       `json:"interval"` // minutes
	CreatedAt time.Time `json:"created_at"`
}

type CheckResult struct {
	Time time.Time `json:"time"`
	Up   bool      `json:"up"`
}

type EndpointStatus struct {
	Up           bool          `json:"up"`
	ResponseMs   int64         `json:"response_ms"`
	LastChecked  time.Time     `json:"last_checked"`
	TotalChecks  int64         `json:"total_checks"`
	SuccessCount int64         `json:"success_count"`
	Error        string        `json:"error,omitempty"`
	History      []CheckResult `json:"history"` // last 24h of checks
}

type EndpointStore struct {
	Endpoints []Endpoint `json:"endpoints"`
}

var (
	endpointsMu       sync.RWMutex
	endpoints         []Endpoint
	endpointStatuses  = make(map[string]*EndpointStatus)
	endpointsFilePath string
)

func init() {
	endpointsFilePath = getEndpointConfigPath()
	loadEndpoints()
	go endpointCheckLoop()
}

func getEndpointConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "/tmp"
	}
	dir := filepath.Join(configDir, "Warden")
	os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "endpoints.json")
}

func loadEndpoints() {
	data, err := os.ReadFile(endpointsFilePath)
	if err != nil {
		return
	}
	var store EndpointStore
	if err := json.Unmarshal(data, &store); err != nil {
		log.Printf("Failed to parse endpoints config: %v", err)
		return
	}
	endpointsMu.Lock()
	endpoints = store.Endpoints
	for _, ep := range endpoints {
		endpointStatuses[ep.ID] = &EndpointStatus{}
	}
	endpointsMu.Unlock()
}

func saveEndpoints() error {
	endpointsMu.RLock()
	store := EndpointStore{Endpoints: endpoints}
	endpointsMu.RUnlock()

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(endpointsFilePath, data, 0600)
}

func endpointCheckLoop() {
	// Initial check
	runEndpointChecks()
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		runEndpointChecks()
	}
}

func runEndpointChecks() {
	endpointsMu.RLock()
	toCheck := make([]Endpoint, 0)
	for _, ep := range endpoints {
		status := endpointStatuses[ep.ID]
		if status == nil || time.Since(status.LastChecked) >= time.Duration(ep.Interval)*time.Minute {
			toCheck = append(toCheck, ep)
		}
	}
	endpointsMu.RUnlock()

	for _, ep := range toCheck {
		up, ms, err := checkEndpoint(ep.Host, ep.Port)
		endpointsMu.Lock()
		status := endpointStatuses[ep.ID]
		if status == nil {
			status = &EndpointStatus{}
			endpointStatuses[ep.ID] = status
		}
		status.Up = up
		status.ResponseMs = ms
		status.LastChecked = time.Now()
		status.TotalChecks++
		if up {
			status.SuccessCount++
			status.Error = ""
		} else if err != nil {
			status.Error = err.Error()
		}
		recordCheck(status, up)
		endpointsMu.Unlock()
	}
}

func checkEndpoint(host string, port int) (bool, int64, error) {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return false, elapsed, err
	}
	conn.Close()
	return true, elapsed, nil
}

func recordCheck(status *EndpointStatus, up bool) {
	status.History = append(status.History, CheckResult{Time: time.Now(), Up: up})
	// Trim to last 12 hours
	cutoff := time.Now().Add(-12 * time.Hour)
	start := 0
	for i, h := range status.History {
		if h.Time.After(cutoff) {
			start = i
			break
		}
	}
	if start > 0 {
		status.History = status.History[start:]
	}
}

// getUptimeBar returns 144 segments (one per 5min) for the last 12h.
// Each segment is: 1=up, 0=down, -1=no data
func getUptimeBar(history []CheckResult) []int {
	segments := make([]int, 144)
	for i := range segments {
		segments[i] = -1
	}
	now := time.Now()
	for _, h := range history {
		age := now.Sub(h.Time)
		if age > 12*time.Hour {
			continue
		}
		idx := 143 - int(age.Minutes()/5)
		if idx < 0 {
			idx = 0
		}
		if idx > 143 {
			idx = 143
		}
		if h.Up {
			if segments[idx] == -1 || segments[idx] == 1 {
				segments[idx] = 1
			} else {
				segments[idx] = 0 // mixed = down takes priority
			}
		} else {
			segments[idx] = 0
		}
	}
	return segments
}

func generateEndpointID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func handleEndpoints(w http.ResponseWriter, r *http.Request) {
	endpointsMu.RLock()
	epCopy := make([]Endpoint, len(endpoints))
	copy(epCopy, endpoints)
	statusCopy := make(map[string]*EndpointStatus)
	for id, s := range endpointStatuses {
		sc := *s
		statusCopy[id] = &sc
	}
	endpointsMu.RUnlock()

	// Compute uptime bars
	uptimeBars := make(map[string][]int)
	for id, s := range statusCopy {
		uptimeBars[id] = getUptimeBar(s.History)
	}

	hd := buildHeaderData(r)
	hd.PageTitle = "Endpoints"
	data := struct {
		HeaderData
		Endpoints  []Endpoint
		Statuses   map[string]*EndpointStatus
		UptimeBars map[string][]int
	}{
		HeaderData: hd,
		Endpoints:  epCopy,
		Statuses:   statusCopy,
		UptimeBars: uptimeBars,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "endpoints.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIEndpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		endpointsMu.RLock()
		type epWithStatus struct {
			Endpoint
			Status *EndpointStatus `json:"status"`
		}
		result := make([]epWithStatus, len(endpoints))
		for i, ep := range endpoints {
			result[i] = epWithStatus{Endpoint: ep, Status: endpointStatuses[ep.ID]}
		}
		endpointsMu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"endpoints": result,
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
	case "add":
		handleEndpointAdd(w, r)
	case "edit":
		handleEndpointEdit(w, r)
	case "delete":
		handleEndpointDelete(w, r)
	case "check_now":
		handleEndpointCheckNow(w, r)
	default:
		respondJSON(w, false, "Invalid action")
	}
}

func handleEndpointAdd(w http.ResponseWriter, r *http.Request) {
	host := strings.TrimSpace(r.FormValue("host"))
	portStr := strings.TrimSpace(r.FormValue("port"))
	intervalStr := strings.TrimSpace(r.FormValue("interval"))
	label := strings.TrimSpace(r.FormValue("label"))

	if host == "" {
		respondJSON(w, false, "Host is required")
		return
	}
	if portStr == "" {
		respondJSON(w, false, "Port is required")
		return
	}

	var port int
	fmt.Sscanf(portStr, "%d", &port)
	if port < 1 || port > 65535 {
		respondJSON(w, false, "Port must be between 1 and 65535")
		return
	}

	var interval int
	fmt.Sscanf(intervalStr, "%d", &interval)
	if interval < 1 {
		interval = 5
	}
	if interval > 1440 {
		respondJSON(w, false, "Interval must be 1440 minutes (24h) or less")
		return
	}

	// Validate host - alphanumeric, dots, hyphens, colons (IPv6)
	for _, ch := range host {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '-' || ch == ':') {
			respondJSON(w, false, "Invalid host format")
			return
		}
	}

	if label == "" {
		label = fmt.Sprintf("%s:%d", host, port)
	}

	ep := Endpoint{
		ID:        generateEndpointID(),
		Host:      host,
		Port:      port,
		Label:     label,
		Interval:  interval,
		CreatedAt: time.Now(),
	}

	endpointsMu.Lock()
	endpoints = append(endpoints, ep)
	endpointStatuses[ep.ID] = &EndpointStatus{}
	endpointsMu.Unlock()

	if err := saveEndpoints(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}

	// Trigger immediate check in background
	go func() {
		up, ms, err := checkEndpoint(ep.Host, ep.Port)
		endpointsMu.Lock()
		status := endpointStatuses[ep.ID]
		if status != nil {
			status.Up = up
			status.ResponseMs = ms
			status.LastChecked = time.Now()
			status.TotalChecks++
			if up {
				status.SuccessCount++
			} else if err != nil {
				status.Error = err.Error()
			}
			recordCheck(status, up)
		}
		endpointsMu.Unlock()
	}()

	auditFromRequest(r, "Endpoint added", fmt.Sprintf("%s:%d", host, port), "endpoint")
	respondJSON(w, true, "Endpoint added")
}

func handleEndpointEdit(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	host := strings.TrimSpace(r.FormValue("host"))
	portStr := strings.TrimSpace(r.FormValue("port"))
	intervalStr := strings.TrimSpace(r.FormValue("interval"))
	label := strings.TrimSpace(r.FormValue("label"))

	if id == "" {
		respondJSON(w, false, "Endpoint ID is required")
		return
	}
	if host == "" {
		respondJSON(w, false, "Host is required")
		return
	}
	if portStr == "" {
		respondJSON(w, false, "Port is required")
		return
	}

	var port int
	fmt.Sscanf(portStr, "%d", &port)
	if port < 1 || port > 65535 {
		respondJSON(w, false, "Port must be between 1 and 65535")
		return
	}

	var interval int
	fmt.Sscanf(intervalStr, "%d", &interval)
	if interval < 1 {
		interval = 5
	}
	if interval > 1440 {
		respondJSON(w, false, "Interval must be 1440 minutes (24h) or less")
		return
	}

	for _, ch := range host {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '-' || ch == ':') {
			respondJSON(w, false, "Invalid host format")
			return
		}
	}

	if label == "" {
		label = fmt.Sprintf("%s:%d", host, port)
	}

	endpointsMu.Lock()
	found := false
	for i := range endpoints {
		if endpoints[i].ID == id {
			endpoints[i].Host = host
			endpoints[i].Port = port
			endpoints[i].Label = label
			endpoints[i].Interval = interval
			found = true
			break
		}
	}
	endpointsMu.Unlock()

	if !found {
		respondJSON(w, false, "Endpoint not found")
		return
	}

	if err := saveEndpoints(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	auditFromRequest(r, "Endpoint edited", fmt.Sprintf("%s:%d", host, port), "endpoint")
	respondJSON(w, true, "Endpoint updated")
}

func handleEndpointDelete(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	if id == "" {
		respondJSON(w, false, "Endpoint ID is required")
		return
	}

	endpointsMu.Lock()
	found := false
	for i, ep := range endpoints {
		if ep.ID == id {
			endpoints = append(endpoints[:i], endpoints[i+1:]...)
			delete(endpointStatuses, id)
			found = true
			break
		}
	}
	endpointsMu.Unlock()

	if !found {
		respondJSON(w, false, "Endpoint not found")
		return
	}

	if err := saveEndpoints(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	auditFromRequest(r, "Endpoint deleted", id, "endpoint")
	respondJSON(w, true, "Endpoint deleted")
}

func handleEndpointCheckNow(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	if id == "" {
		respondJSON(w, false, "Endpoint ID is required")
		return
	}

	endpointsMu.RLock()
	var ep *Endpoint
	for i := range endpoints {
		if endpoints[i].ID == id {
			ep = &endpoints[i]
			break
		}
	}
	endpointsMu.RUnlock()

	if ep == nil {
		respondJSON(w, false, "Endpoint not found")
		return
	}

	up, ms, err := checkEndpoint(ep.Host, ep.Port)

	endpointsMu.Lock()
	status := endpointStatuses[id]
	if status == nil {
		status = &EndpointStatus{}
		endpointStatuses[id] = status
	}
	status.Up = up
	status.ResponseMs = ms
	status.LastChecked = time.Now()
	status.TotalChecks++
	if up {
		status.SuccessCount++
		status.Error = ""
	} else if err != nil {
		status.Error = err.Error()
	}
	recordCheck(status, up)
	endpointsMu.Unlock()

	if up {
		respondJSON(w, true, fmt.Sprintf("Up (%dms)", ms))
	} else {
		respondJSON(w, false, fmt.Sprintf("Down: %s", status.Error))
	}
}
