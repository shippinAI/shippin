package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// WardenConfig is the combined export format for all Warden settings.
type WardenConfig struct {
	ExportedAt        string             `json:"exported_at"`
	WardenVersion     string             `json:"warden_version"`
	Endpoints         []Endpoint         `json:"endpoints"`
	Notifications     NotificationConfig `json:"notifications"`
	Backups           BackupStore        `json:"backups"`
	PlatformSystems   []PlatformSystem   `json:"platform_systems,omitempty"`
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	hd := buildHeaderData(r)
	hd.PageTitle = "Config"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "config.html", hd); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleConfigExport(w, r)
	case http.MethodPost:
		handleConfigImport(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleConfigExport(w http.ResponseWriter, r *http.Request) {
	endpointsMu.RLock()
	eps := make([]Endpoint, len(endpoints))
	copy(eps, endpoints)
	endpointsMu.RUnlock()

	notifMu.RLock()
	notif := NotificationConfig{
		Channels: notifConfig.Channels,
		Rules:    notifConfig.Rules,
	}
	notifMu.RUnlock()

	backupMu.RLock()
	bk := BackupStore{
		Jobs:   backupStore.Jobs,
		Estate: backupStore.Estate,
	}
	backupMu.RUnlock()

	plat := listPlatformSystems()

	cfg := WardenConfig{
		ExportedAt:      time.Now().Format("2006-01-02T15:04:05Z"),
		WardenVersion:   Version,
		Endpoints:       eps,
		Notifications:   notif,
		Backups:         bk,
		PlatformSystems: plat,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		http.Error(w, "Failed to export config", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("Warden-config-%s.json", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Write(data)

	auditFromRequest(r, "config_export", "Exported configuration", "system")
}

func handleConfigImport(w http.ResponseWriter, r *http.Request) {
	// Limit upload size to 10MB
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	file, _, err := r.FormFile("config")
	if err != nil {
		jsonError(w, "No config file provided")
		return
	}
	defer file.Close()

	var cfg WardenConfig
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		jsonError(w, "Invalid config file: "+err.Error())
		return
	}

	mode := r.FormValue("mode") // "merge" or "replace"
	if mode == "" {
		mode = "replace"
	}

	imported := struct {
		Endpoints        int `json:"endpoints"`
		Channels         int `json:"channels"`
		Rules            int `json:"rules"`
		BackupJobs       int `json:"backup_jobs"`
		PlatformSystemsN int `json:"platform_systems"`
	}{}

	// Import endpoints
	if cfg.Endpoints != nil {
		endpointsMu.Lock()
		if mode == "replace" {
			endpoints = cfg.Endpoints
		} else {
			existing := make(map[string]bool)
			for _, ep := range endpoints {
				existing[ep.ID] = true
			}
			for _, ep := range cfg.Endpoints {
				if !existing[ep.ID] {
					endpoints = append(endpoints, ep)
				}
			}
		}
		imported.Endpoints = len(cfg.Endpoints)
		endpointsMu.Unlock()
		saveEndpoints()
	}

	// Import notifications
	if cfg.Notifications.Channels != nil || cfg.Notifications.Rules != nil {
		notifMu.Lock()
		if mode == "replace" {
			notifConfig.Channels = cfg.Notifications.Channels
			notifConfig.Rules = cfg.Notifications.Rules
		} else {
			existingCh := make(map[string]bool)
			for _, ch := range notifConfig.Channels {
				existingCh[ch.ID] = true
			}
			for _, ch := range cfg.Notifications.Channels {
				if !existingCh[ch.ID] {
					notifConfig.Channels = append(notifConfig.Channels, ch)
				}
			}
			existingRl := make(map[string]bool)
			for _, rl := range notifConfig.Rules {
				existingRl[rl.ID] = true
			}
			for _, rl := range cfg.Notifications.Rules {
				if !existingRl[rl.ID] {
					notifConfig.Rules = append(notifConfig.Rules, rl)
				}
			}
		}
		if cfg.Notifications.Channels != nil {
			imported.Channels = len(cfg.Notifications.Channels)
		}
		if cfg.Notifications.Rules != nil {
			imported.Rules = len(cfg.Notifications.Rules)
		}
		notifMu.Unlock()
		saveNotifConfig()
	}

	// Import backup jobs
	if cfg.Backups.Jobs != nil {
		backupMu.Lock()
		if mode == "replace" {
			backupStore.Jobs = cfg.Backups.Jobs
		} else {
			existing := make(map[string]bool)
			for _, j := range backupStore.Jobs {
				existing[j.ID] = true
			}
			for _, j := range cfg.Backups.Jobs {
				if !existing[j.ID] {
					backupStore.Jobs = append(backupStore.Jobs, j)
				}
			}
		}
		imported.BackupJobs = len(cfg.Backups.Jobs)
		if cfg.Backups.Estate.Destination != "" || len(cfg.Backups.Estate.Targets) > 0 {
			backupStore.Estate = cfg.Backups.Estate
		}
		backupMu.Unlock()
		saveBackupStore()
	} else if cfg.Backups.Estate.Destination != "" || len(cfg.Backups.Estate.Targets) > 0 {
		backupMu.Lock()
		backupStore.Estate = cfg.Backups.Estate
		backupMu.Unlock()
		saveBackupStore()
	}

	if cfg.PlatformSystems != nil {
		if mode == "replace" {
			if err := replacePlatformSystems(cfg.PlatformSystems); err != nil {
				jsonError(w, "platform systems: "+err.Error())
				return
			}
		} else {
			if err := mergePlatformSystemsExtra(cfg.PlatformSystems); err != nil {
				jsonError(w, "platform systems: "+err.Error())
				return
			}
		}
		imported.PlatformSystemsN = len(cfg.PlatformSystems)
	}

	detail := fmt.Sprintf("Imported config (%s): %d endpoints, %d channels, %d rules, %d backup jobs, %d platform systems",
		mode, imported.Endpoints, imported.Channels, imported.Rules, imported.BackupJobs, imported.PlatformSystemsN)
	auditFromRequest(r, "config_import", detail, "system")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"mode":     mode,
		"imported": imported,
	})
}
