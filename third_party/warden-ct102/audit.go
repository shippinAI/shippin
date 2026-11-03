package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxAuditEntries = 500

type AuditEntry struct {
	Time     string `json:"time"`
	User     string `json:"user"`
	Action   string `json:"action"`
	Detail   string `json:"detail,omitempty"`
	IP       string `json:"ip"`
	Category string `json:"category"` // auth, service, docker, file, system, firewall, cron, user, dns, endpoint, sshkey, package
}

var (
	auditLog   []AuditEntry
	auditMu    sync.RWMutex
	auditPath  string
)

func initAudit() {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "/tmp"
	}
	auditPath = filepath.Join(dir, "warden", "audit.json")
	loadAudit()
}

func loadAudit() {
	data, err := os.ReadFile(auditPath)
	if err != nil {
		return
	}
	auditMu.Lock()
	defer auditMu.Unlock()
	json.Unmarshal(data, &auditLog)
}

func saveAudit() {
	auditMu.RLock()
	data, err := json.MarshalIndent(auditLog, "", "  ")
	auditMu.RUnlock()
	if err != nil {
		log.Printf("audit save error: %v", err)
		return
	}
	os.MkdirAll(filepath.Dir(auditPath), 0700)
	os.WriteFile(auditPath, data, 0600)
}

func addAuditEntry(user, action, detail, ip, category string) {
	entry := AuditEntry{
		Time:     time.Now().Format("2006-01-02 15:04:05"),
		User:     user,
		Action:   action,
		Detail:   detail,
		IP:       ip,
		Category: category,
	}

	auditMu.Lock()
	auditLog = append([]AuditEntry{entry}, auditLog...)
	if len(auditLog) > maxAuditEntries {
		auditLog = auditLog[:maxAuditEntries]
	}
	auditMu.Unlock()

	go saveAudit()
}

// auditFromRequest is a helper that extracts user/IP from the request
func auditFromRequest(r *http.Request, action, detail, category string) {
	user, _ := getSession(r)
	if user == "" {
		user = "anonymous"
	}
	ip := getClientIP(r)
	addAuditEntry(user, action, detail, ip, category)
}

type AuditPageData struct {
	HeaderData
	Entries []AuditEntry
}

func handleAudit(w http.ResponseWriter, r *http.Request) {
	hd := buildHeaderData(r)
	hd.PageTitle = "Audit Log"

	auditMu.RLock()
	entries := make([]AuditEntry, len(auditLog))
	copy(entries, auditLog)
	auditMu.RUnlock()

	data := AuditPageData{
		HeaderData: hd,
		Entries:    entries,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "audit.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		auditMu.Lock()
		auditLog = nil
		auditMu.Unlock()
		go saveAudit()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
		return
	}

	category := r.URL.Query().Get("category")

	auditMu.RLock()
	var filtered []AuditEntry
	for _, e := range auditLog {
		if category != "" && e.Category != category {
			continue
		}
		filtered = append(filtered, e)
	}
	auditMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"entries": filtered,
		"total":   len(filtered),
	})
}

// Helper for formatting audit details
func auditDetail(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}
