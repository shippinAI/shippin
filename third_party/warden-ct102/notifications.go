package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ── Data models ──

type NotificationChannel struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Type    string      `json:"type"` // webhook, discord, slack, email
	URL     string      `json:"url,omitempty"`
	SMTP    *SMTPConfig `json:"smtp,omitempty"`
	Enabled bool        `json:"enabled"`
}

type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
	UseTLS   bool   `json:"useTLS"`
}

type AlertRule struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`      // endpoint_down, cert_expiring, disk_full, service_stopped
	Threshold int    `json:"threshold"` // consecutive failures, days until expiry, disk % usage
	ChannelID string `json:"channelId"`
	Cooldown  int    `json:"cooldown"` // minutes between repeat alerts
	Enabled   bool   `json:"enabled"`
}

type AlertEvent struct {
	Time     string `json:"time"`
	RuleName string `json:"ruleName"`
	RuleType string `json:"ruleType"`
	Channel  string `json:"channel"`
	Message  string `json:"message"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

type NotificationConfig struct {
	Channels []NotificationChannel `json:"channels"`
	Rules    []AlertRule           `json:"rules"`
	Events   []AlertEvent          `json:"events"`
}

var (
	notifMu     sync.RWMutex
	notifConfig NotificationConfig
	notifPath   string
	lastAlerted = make(map[string]time.Time) // ruleID:detail -> last alert time
)

const maxAlertEvents = 200

func init() {
	notifPath = getNotifConfigPath()
	loadNotifConfig()
	go alertCheckLoop()
}

func getNotifConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "/tmp"
	}
	dir := filepath.Join(configDir, "Warden")
	os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "notifications.json")
}

func loadNotifConfig() {
	data, err := os.ReadFile(notifPath)
	if err != nil {
		return
	}
	notifMu.Lock()
	defer notifMu.Unlock()
	if err := json.Unmarshal(data, &notifConfig); err != nil {
		log.Printf("Failed to parse notifications config: %v", err)
	}
}

func saveNotifConfig() error {
	notifMu.RLock()
	data, err := json.MarshalIndent(notifConfig, "", "  ")
	notifMu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(notifPath, data, 0600)
}

// ── Alert check loop ──

func alertCheckLoop() {
	// Wait for initial data to populate
	time.Sleep(30 * time.Second)
	for {
		checkAllAlertRules()
		time.Sleep(60 * time.Second)
	}
}

func checkAllAlertRules() {
	notifMu.RLock()
	rules := make([]AlertRule, len(notifConfig.Rules))
	copy(rules, notifConfig.Rules)
	channels := make(map[string]NotificationChannel)
	for _, ch := range notifConfig.Channels {
		channels[ch.ID] = ch
	}
	notifMu.RUnlock()

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		ch, ok := channels[rule.ChannelID]
		if !ok || !ch.Enabled {
			continue
		}

		switch rule.Type {
		case "endpoint_down":
			checkEndpointAlerts(rule, ch)
		case "cert_expiring":
			checkCertAlerts(rule, ch)
		case "disk_full":
			checkDiskAlerts(rule, ch)
		}
	}
}

func checkEndpointAlerts(rule AlertRule, ch NotificationChannel) {
	endpointsMu.RLock()
	defer endpointsMu.RUnlock()

	for _, ep := range endpoints {
		status := endpointStatuses[ep.ID]
		if status == nil || status.TotalChecks == 0 {
			continue
		}
		if !status.Up {
			// Count consecutive failures from history
			consecutive := 0
			for i := len(status.History) - 1; i >= 0; i-- {
				if !status.History[i].Up {
					consecutive++
				} else {
					break
				}
			}
			if consecutive >= rule.Threshold {
				key := rule.ID + ":" + ep.ID
				if shouldAlert(key, rule.Cooldown) {
					msg := fmt.Sprintf("Endpoint DOWN: %s (%s:%d) — %d consecutive failures",
						ep.Label, ep.Host, ep.Port, consecutive)
					sendAlert(rule, ch, msg)
				}
			}
		}
	}
}

func checkCertAlerts(rule AlertRule, ch NotificationChannel) {
	certs := scanCertificates()
	for _, cert := range certs {
		if cert.DaysLeft <= rule.Threshold && cert.DaysLeft >= 0 {
			key := rule.ID + ":" + cert.Domain
			if shouldAlert(key, rule.Cooldown) {
				msg := fmt.Sprintf("Certificate EXPIRING: %s — %d days remaining (expires %s)",
					cert.Domain, cert.DaysLeft, cert.NotAfter)
				sendAlert(rule, ch, msg)
			}
		} else if cert.DaysLeft < 0 {
			key := rule.ID + ":" + cert.Domain + ":expired"
			if shouldAlert(key, rule.Cooldown) {
				msg := fmt.Sprintf("Certificate EXPIRED: %s — expired on %s",
					cert.Domain, cert.NotAfter)
				sendAlert(rule, ch, msg)
			}
		}
	}
}

func checkDiskAlerts(rule AlertRule, ch NotificationChannel) {
	stats := collectStats()
	for _, disk := range stats.Disks {
		if int(disk.UsedPerc) >= rule.Threshold {
			key := rule.ID + ":" + disk.MountPoint
			if shouldAlert(key, rule.Cooldown) {
				msg := fmt.Sprintf("Disk FULL: %s at %.0f%% usage",
					disk.MountPoint, disk.UsedPerc)
				sendAlert(rule, ch, msg)
			}
		}
	}
}

func shouldAlert(key string, cooldownMin int) bool {
	notifMu.RLock()
	last, exists := lastAlerted[key]
	notifMu.RUnlock()
	if !exists {
		return true
	}
	return time.Since(last) >= time.Duration(cooldownMin)*time.Minute
}

func sendAlert(rule AlertRule, ch NotificationChannel, message string) {
	var err error

	switch ch.Type {
	case "webhook":
		err = sendWebhook(ch.URL, message)
	case "discord":
		err = sendDiscordWebhook(ch.URL, message)
	case "slack":
		err = sendSlackWebhook(ch.URL, message)
	case "email":
		if ch.SMTP != nil {
			err = sendEmail(ch.SMTP, message)
		} else {
			err = fmt.Errorf("no SMTP config")
		}
	default:
		err = fmt.Errorf("unknown channel type: %s", ch.Type)
	}

	event := AlertEvent{
		Time:     time.Now().Format("2006-01-02 15:04:05"),
		RuleName: rule.Name,
		RuleType: rule.Type,
		Channel:  ch.Name,
		Message:  message,
		Success:  err == nil,
	}
	if err != nil {
		event.Error = err.Error()
		log.Printf("Alert send failed [%s -> %s]: %v", rule.Name, ch.Name, err)
	} else {
		log.Printf("Alert sent [%s -> %s]: %s", rule.Name, ch.Name, message)
	}

	notifMu.Lock()
	lastAlerted[rule.ID+":"+message[:min(len(message), 40)]] = time.Now()
	notifConfig.Events = append([]AlertEvent{event}, notifConfig.Events...)
	if len(notifConfig.Events) > maxAlertEvents {
		notifConfig.Events = notifConfig.Events[:maxAlertEvents]
	}
	notifMu.Unlock()

	saveNotifConfig()
}

// ── Senders ──

func sendWebhook(url, message string) error {
	payload := map[string]string{"text": message, "message": message}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func sendDiscordWebhook(url, message string) error {
	payload := map[string]interface{}{
		"content": "",
		"embeds": []map[string]interface{}{
			{
				"title":       "Warden Alert",
				"description": message,
				"color":       16711680, // red
				"timestamp":   time.Now().Format(time.RFC3339),
				"footer":      map[string]string{"text": "Warden Server Panel"},
			},
		},
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func sendSlackWebhook(url, message string) error {
	payload := map[string]interface{}{
		"text": message,
		"blocks": []map[string]interface{}{
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": "*Warden Alert*\n" + message,
				},
			},
		},
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func sendEmail(cfg *SMTPConfig, message string) error {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	subject := "Warden Alert"
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		cfg.From, cfg.To, subject, message)

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	return smtp.SendMail(addr, auth, cfg.From, strings.Split(cfg.To, ","), []byte(body))
}

// ── HTTP handlers ──

// TimelineDay holds aggregated alert counts for a single day.
type TimelineDay struct {
	Date             string // "2006-01-02"
	Label            string // "Jan 02"
	Total            int
	EndpointDown     int
	CertExpiring     int
	DiskFull         int
	Failed           int
	EndpointDownPerc float64 // percentage of max for bar height
	CertExpiringPerc float64
	DiskFullPerc     float64
}

func buildTimeline(events []AlertEvent) []TimelineDay {
	const days = 30
	now := time.Now()
	dayMap := make(map[string]*TimelineDay)

	// Pre-fill last 30 days
	timeline := make([]TimelineDay, days)
	for i := 0; i < days; i++ {
		d := now.AddDate(0, 0, -(days - 1 - i))
		key := d.Format("2006-01-02")
		timeline[i] = TimelineDay{
			Date:  key,
			Label: d.Format("Jan 02"),
		}
		dayMap[key] = &timeline[i]
	}

	// Aggregate events into days
	for _, ev := range events {
		t, err := time.Parse("2006-01-02 15:04:05", ev.Time)
		if err != nil {
			continue
		}
		key := t.Format("2006-01-02")
		td, ok := dayMap[key]
		if !ok {
			continue // older than 30 days
		}
		td.Total++
		if !ev.Success {
			td.Failed++
		}
		switch ev.RuleType {
		case "endpoint_down":
			td.EndpointDown++
		case "cert_expiring":
			td.CertExpiring++
		case "disk_full":
			td.DiskFull++
		}
	}

	// Compute max for bar heights
	maxVal := 0
	for _, td := range timeline {
		if td.Total > maxVal {
			maxVal = td.Total
		}
	}
	if maxVal > 0 {
		for i := range timeline {
			timeline[i].EndpointDownPerc = float64(timeline[i].EndpointDown) / float64(maxVal) * 100
			timeline[i].CertExpiringPerc = float64(timeline[i].CertExpiring) / float64(maxVal) * 100
			timeline[i].DiskFullPerc = float64(timeline[i].DiskFull) / float64(maxVal) * 100
		}
	}

	return timeline
}

func handleNotifications(w http.ResponseWriter, r *http.Request) {
	notifMu.RLock()
	cfg := notifConfig
	// Copy events for timeline
	events := make([]AlertEvent, len(notifConfig.Events))
	copy(events, notifConfig.Events)
	notifMu.RUnlock()

	timeline := buildTimeline(events)

	hd := buildHeaderData(r)
	hd.PageTitle = "Notifications"
	data := struct {
		HeaderData
		Channels []NotificationChannel
		Rules    []AlertRule
		Events   []AlertEvent
		Timeline []TimelineDay
	}{
		HeaderData: hd,
		Channels:   cfg.Channels,
		Rules:      cfg.Rules,
		Events:     cfg.Events,
		Timeline:   timeline,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "notifications.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPINotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		notifMu.RLock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(notifConfig)
		notifMu.RUnlock()
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.ParseForm()
	action := r.FormValue("action")

	switch action {
	case "add_channel":
		handleAddChannel(w, r)
	case "edit_channel":
		handleEditChannel(w, r)
	case "delete_channel":
		handleDeleteChannel(w, r)
	case "test_channel":
		handleTestChannel(w, r)
	case "toggle_channel":
		handleToggleChannel(w, r)
	case "add_rule":
		handleAddRule(w, r)
	case "edit_rule":
		handleEditRule(w, r)
	case "delete_rule":
		handleDeleteRule(w, r)
	case "toggle_rule":
		handleToggleRule(w, r)
	case "clear_events":
		handleClearEvents(w, r)
	default:
		respondJSON(w, false, "Invalid action")
	}
}

func handleAddChannel(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	chType := strings.TrimSpace(r.FormValue("type"))
	url := strings.TrimSpace(r.FormValue("url"))

	if name == "" {
		respondJSON(w, false, "Name is required")
		return
	}
	if chType == "" {
		respondJSON(w, false, "Type is required")
		return
	}

	ch := NotificationChannel{
		ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:    name,
		Type:    chType,
		URL:     url,
		Enabled: true,
	}

	if chType == "email" {
		portStr := r.FormValue("smtp_port")
		var port int
		fmt.Sscanf(portStr, "%d", &port)
		if port == 0 {
			port = 587
		}
		ch.SMTP = &SMTPConfig{
			Host:     strings.TrimSpace(r.FormValue("smtp_host")),
			Port:     port,
			Username: strings.TrimSpace(r.FormValue("smtp_user")),
			Password: strings.TrimSpace(r.FormValue("smtp_pass")),
			From:     strings.TrimSpace(r.FormValue("smtp_from")),
			To:       strings.TrimSpace(r.FormValue("smtp_to")),
			UseTLS:   r.FormValue("smtp_tls") == "on",
		}
	}

	notifMu.Lock()
	notifConfig.Channels = append(notifConfig.Channels, ch)
	notifMu.Unlock()

	if err := saveNotifConfig(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	auditFromRequest(r, "Notification channel added", name+" ("+chType+")", "system")
	respondJSON(w, true, "Channel added")
}

func handleEditChannel(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	name := strings.TrimSpace(r.FormValue("name"))
	url := strings.TrimSpace(r.FormValue("url"))

	notifMu.Lock()
	found := false
	for i := range notifConfig.Channels {
		if notifConfig.Channels[i].ID == id {
			notifConfig.Channels[i].Name = name
			notifConfig.Channels[i].URL = url
			if notifConfig.Channels[i].Type == "email" {
				portStr := r.FormValue("smtp_port")
				var port int
				fmt.Sscanf(portStr, "%d", &port)
				if port == 0 {
					port = 587
				}
				notifConfig.Channels[i].SMTP = &SMTPConfig{
					Host:     strings.TrimSpace(r.FormValue("smtp_host")),
					Port:     port,
					Username: strings.TrimSpace(r.FormValue("smtp_user")),
					Password: strings.TrimSpace(r.FormValue("smtp_pass")),
					From:     strings.TrimSpace(r.FormValue("smtp_from")),
					To:       strings.TrimSpace(r.FormValue("smtp_to")),
					UseTLS:   r.FormValue("smtp_tls") == "on",
				}
			}
			found = true
			break
		}
	}
	notifMu.Unlock()

	if !found {
		respondJSON(w, false, "Channel not found")
		return
	}
	if err := saveNotifConfig(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	respondJSON(w, true, "Channel updated")
}

func handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	notifMu.Lock()
	found := false
	for i, ch := range notifConfig.Channels {
		if ch.ID == id {
			notifConfig.Channels = append(notifConfig.Channels[:i], notifConfig.Channels[i+1:]...)
			found = true
			break
		}
	}
	notifMu.Unlock()

	if !found {
		respondJSON(w, false, "Channel not found")
		return
	}
	if err := saveNotifConfig(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	auditFromRequest(r, "Notification channel deleted", id, "system")
	respondJSON(w, true, "Channel deleted")
}

func handleTestChannel(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	notifMu.RLock()
	var ch *NotificationChannel
	for i := range notifConfig.Channels {
		if notifConfig.Channels[i].ID == id {
			ch = &notifConfig.Channels[i]
			break
		}
	}
	notifMu.RUnlock()

	if ch == nil {
		respondJSON(w, false, "Channel not found")
		return
	}

	testMsg := "This is a test alert from Warden Server Panel."
	var err error
	switch ch.Type {
	case "webhook":
		err = sendWebhook(ch.URL, testMsg)
	case "discord":
		err = sendDiscordWebhook(ch.URL, testMsg)
	case "slack":
		err = sendSlackWebhook(ch.URL, testMsg)
	case "email":
		if ch.SMTP != nil {
			err = sendEmail(ch.SMTP, testMsg)
		} else {
			err = fmt.Errorf("no SMTP configuration")
		}
	}

	if err != nil {
		respondJSON(w, false, "Test failed: "+err.Error())
	} else {
		respondJSON(w, true, "Test sent successfully")
	}
}

func handleToggleChannel(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	notifMu.Lock()
	for i := range notifConfig.Channels {
		if notifConfig.Channels[i].ID == id {
			notifConfig.Channels[i].Enabled = !notifConfig.Channels[i].Enabled
			break
		}
	}
	notifMu.Unlock()
	saveNotifConfig()
	respondJSON(w, true, "Channel toggled")
}

func handleAddRule(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	ruleType := strings.TrimSpace(r.FormValue("type"))
	channelID := strings.TrimSpace(r.FormValue("channel_id"))
	thresholdStr := r.FormValue("threshold")
	cooldownStr := r.FormValue("cooldown")

	if name == "" || ruleType == "" || channelID == "" {
		respondJSON(w, false, "Name, type, and channel are required")
		return
	}

	var threshold int
	fmt.Sscanf(thresholdStr, "%d", &threshold)
	if threshold < 1 {
		threshold = getDefaultThreshold(ruleType)
	}

	var cooldown int
	fmt.Sscanf(cooldownStr, "%d", &cooldown)
	if cooldown < 1 {
		cooldown = 30
	}

	rule := AlertRule{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:      name,
		Type:      ruleType,
		Threshold: threshold,
		ChannelID: channelID,
		Cooldown:  cooldown,
		Enabled:   true,
	}

	notifMu.Lock()
	notifConfig.Rules = append(notifConfig.Rules, rule)
	notifMu.Unlock()

	if err := saveNotifConfig(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	auditFromRequest(r, "Alert rule added", name+" ("+ruleType+")", "system")
	respondJSON(w, true, "Rule added")
}

func handleEditRule(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	name := strings.TrimSpace(r.FormValue("name"))
	channelID := strings.TrimSpace(r.FormValue("channel_id"))
	thresholdStr := r.FormValue("threshold")
	cooldownStr := r.FormValue("cooldown")

	notifMu.Lock()
	found := false
	for i := range notifConfig.Rules {
		if notifConfig.Rules[i].ID == id {
			if name != "" {
				notifConfig.Rules[i].Name = name
			}
			if channelID != "" {
				notifConfig.Rules[i].ChannelID = channelID
			}
			var threshold int
			fmt.Sscanf(thresholdStr, "%d", &threshold)
			if threshold > 0 {
				notifConfig.Rules[i].Threshold = threshold
			}
			var cooldown int
			fmt.Sscanf(cooldownStr, "%d", &cooldown)
			if cooldown > 0 {
				notifConfig.Rules[i].Cooldown = cooldown
			}
			found = true
			break
		}
	}
	notifMu.Unlock()

	if !found {
		respondJSON(w, false, "Rule not found")
		return
	}
	if err := saveNotifConfig(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	respondJSON(w, true, "Rule updated")
}

func handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	notifMu.Lock()
	found := false
	for i, rule := range notifConfig.Rules {
		if rule.ID == id {
			notifConfig.Rules = append(notifConfig.Rules[:i], notifConfig.Rules[i+1:]...)
			found = true
			break
		}
	}
	notifMu.Unlock()

	if !found {
		respondJSON(w, false, "Rule not found")
		return
	}
	if err := saveNotifConfig(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	auditFromRequest(r, "Alert rule deleted", id, "system")
	respondJSON(w, true, "Rule deleted")
}

func handleToggleRule(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	notifMu.Lock()
	for i := range notifConfig.Rules {
		if notifConfig.Rules[i].ID == id {
			notifConfig.Rules[i].Enabled = !notifConfig.Rules[i].Enabled
			break
		}
	}
	notifMu.Unlock()
	saveNotifConfig()
	respondJSON(w, true, "Rule toggled")
}

func handleClearEvents(w http.ResponseWriter, r *http.Request) {
	notifMu.Lock()
	notifConfig.Events = nil
	notifMu.Unlock()
	saveNotifConfig()
	auditFromRequest(r, "Alert history cleared", "", "system")
	respondJSON(w, true, "History cleared")
}

func getDefaultThreshold(ruleType string) int {
	switch ruleType {
	case "endpoint_down":
		return 3
	case "cert_expiring":
		return 30
	case "disk_full":
		return 90
	default:
		return 1
	}
}
