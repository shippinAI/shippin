package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ── Data models ──

type BackupJob struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`        // tar, rsync
	Source      string `json:"source"`      // source path
	Destination string `json:"destination"` // destination path
	Schedule    string `json:"schedule"`    // cron expression or "manual"
	Compress    bool   `json:"compress"`    // gzip compression (tar only)
	Exclude     string `json:"exclude"`     // comma-separated exclude patterns
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"createdAt"`
}

type BackupRun struct {
	ID           string `json:"id"`
	JobID        string `json:"jobId"`
	JobName      string `json:"jobName"`
	Kind         string `json:"kind,omitempty"` // job, estate
	Type         string `json:"type"`
	Source       string `json:"source"`
	Dest         string `json:"dest"`
	Status       string `json:"status"` // running, success, failed
	StartTime    string `json:"startTime"`
	EndTime      string `json:"endTime,omitempty"`
	Duration     string `json:"duration,omitempty"`
	Size         string `json:"size,omitempty"`
	Output       string `json:"output,omitempty"`
	Error        string `json:"error,omitempty"`
	FilePath     string `json:"filePath,omitempty"`
	ManifestPath string `json:"manifestPath,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	FileCount    int    `json:"fileCount,omitempty"`
}

type BackupEvent struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Action  string `json:"action"`
	Message string `json:"message"`
}

type BackupCheck struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
	Status string `json:"status"`
}

type BackupStore struct {
	Jobs    []BackupJob        `json:"jobs"`
	History []BackupRun        `json:"history"`
	Estate  EstateBackupConfig `json:"estate"`
	Events  []BackupEvent      `json:"events,omitempty"`
}

var (
	backupMu    sync.RWMutex
	backupStore BackupStore
	backupPath  string
	runningJobs = make(map[string]bool) // jobID -> running
)

const maxBackupHistory = 100

func init() {
	backupPath = getBackupConfigPath()
	loadBackupStore()
	go backupScheduleLoop()
}

func getBackupConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "/tmp"
	}
	dir := filepath.Join(configDir, "Warden")
	os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "backups.json")
}

func loadBackupStore() {
	data, err := os.ReadFile(backupPath)
	backupMu.Lock()
	if err == nil {
		if err := json.Unmarshal(data, &backupStore); err != nil {
			log.Printf("Failed to parse backups config: %v", err)
		}
	}
	changed := seedEstateDefaultsLocked()
	backupMu.Unlock()
	loadDestSecrets()
	if changed {
		if err := saveBackupStore(); err != nil {
			log.Printf("Failed to save estate backup defaults: %v", err)
		}
	}
}

func saveBackupStore() error {
	backupMu.RLock()
	data, err := json.MarshalIndent(backupStore, "", "  ")
	backupMu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(backupPath, data, 0600)
}

// ── Schedule loop ──

func backupScheduleLoop() {
	for {
		time.Sleep(60 * time.Second)
		now := time.Now()
		backupMu.RLock()
		jobs := make([]BackupJob, len(backupStore.Jobs))
		copy(jobs, backupStore.Jobs)
		backupMu.RUnlock()

		for _, job := range jobs {
			if !job.Enabled || job.Schedule == "manual" || job.Schedule == "" {
				continue
			}
			if shouldRunSchedule(job.Schedule, now) && !isJobRunning(job.ID) {
				go executeBackup(job)
			}
		}
	}
}

// shouldRunSchedule checks simple schedule strings like "daily", "hourly", "weekly"
// or cron-like "HH:MM" for daily at specific time
func shouldRunSchedule(schedule string, now time.Time) bool {
	switch strings.ToLower(schedule) {
	case "hourly":
		return now.Minute() == 0
	case "daily":
		return now.Hour() == 2 && now.Minute() == 0 // 2:00 AM
	case "weekly":
		return now.Weekday() == time.Sunday && now.Hour() == 2 && now.Minute() == 0
	case "monthly":
		return now.Day() == 1 && now.Hour() == 2 && now.Minute() == 0
	default:
		// Try HH:MM format for daily at specific time
		if len(schedule) == 5 && schedule[2] == ':' {
			return fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute()) == schedule
		}
		return false
	}
}

// ── Backup execution ──

func executeBackup(job BackupJob) {
	backupMu.Lock()
	runningJobs[job.ID] = true
	backupMu.Unlock()

	defer func() {
		backupMu.Lock()
		delete(runningJobs, job.ID)
		backupMu.Unlock()
	}()

	run := BackupRun{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		JobID:     job.ID,
		JobName:   job.Name,
		Type:      job.Type,
		Source:    job.Source,
		Dest:      job.Destination,
		Status:    "running",
		StartTime: time.Now().Format("2006-01-02 15:04:05"),
	}

	// Add to history immediately as "running"
	backupMu.Lock()
	backupStore.History = append([]BackupRun{run}, backupStore.History...)
	if len(backupStore.History) > maxBackupHistory {
		backupStore.History = backupStore.History[:maxBackupHistory]
	}
	backupMu.Unlock()
	saveBackupStore()

	start := time.Now()
	var output string
	var err error
	var filePath string

	switch job.Type {
	case "tar":
		filePath, output, err = executeTarBackup(job)
	case "rsync":
		output, err = executeRsyncBackup(job)
	default:
		err = fmt.Errorf("unknown backup type: %s", job.Type)
	}

	elapsed := time.Since(start)

	// Update run result
	backupMu.Lock()
	for i := range backupStore.History {
		if backupStore.History[i].ID == run.ID {
			backupStore.History[i].EndTime = time.Now().Format("2006-01-02 15:04:05")
			backupStore.History[i].Duration = formatDuration(elapsed)
			backupStore.History[i].Output = output
			backupStore.History[i].FilePath = filePath
			if err != nil {
				backupStore.History[i].Status = "failed"
				backupStore.History[i].Error = err.Error()
			} else {
				backupStore.History[i].Status = "success"
				// Get size of backup
				if filePath != "" {
					if info, serr := os.Stat(filePath); serr == nil {
						backupStore.History[i].Size = formatBytes(uint64(info.Size()))
					}
				}
			}
			break
		}
	}
	backupMu.Unlock()
	saveBackupStore()

	if err != nil {
		log.Printf("Backup failed [%s]: %v", job.Name, err)
	} else {
		log.Printf("Backup completed [%s] in %s", job.Name, formatDuration(elapsed))
	}
}

func executeTarBackup(job BackupJob) (string, string, error) {
	// Ensure destination directory exists
	if err := os.MkdirAll(job.Destination, 0755); err != nil {
		return "", "", fmt.Errorf("create dest dir: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	baseName := strings.ReplaceAll(filepath.Base(job.Source), "/", "_")
	if baseName == "" || baseName == "." {
		baseName = "backup"
	}

	var filename string
	var args []string

	if job.Compress {
		filename = fmt.Sprintf("%s-%s.tar.gz", baseName, timestamp)
		args = []string{"-czf", filepath.Join(job.Destination, filename)}
	} else {
		filename = fmt.Sprintf("%s-%s.tar", baseName, timestamp)
		args = []string{"-cf", filepath.Join(job.Destination, filename)}
	}

	// Add exclude patterns
	if job.Exclude != "" {
		for _, pattern := range strings.Split(job.Exclude, ",") {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" {
				args = append(args, "--exclude="+pattern)
			}
		}
	}

	args = append(args, "-C", filepath.Dir(job.Source), filepath.Base(job.Source))

	cmd := exec.Command("tar", args...)
	out, err := cmd.CombinedOutput()
	outStr := string(out)
	if len(outStr) > 2000 {
		outStr = outStr[:2000] + "\n... (truncated)"
	}

	filePath := filepath.Join(job.Destination, filename)
	return filePath, outStr, err
}

func executeRsyncBackup(job BackupJob) (string, error) {
	// Ensure destination directory exists
	if err := os.MkdirAll(job.Destination, 0755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	args := []string{"-av", "--delete"}

	// Add exclude patterns
	if job.Exclude != "" {
		for _, pattern := range strings.Split(job.Exclude, ",") {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" {
				args = append(args, "--exclude="+pattern)
			}
		}
	}

	// Ensure source has trailing slash for rsync
	source := job.Source
	if !strings.HasSuffix(source, "/") {
		source += "/"
	}

	args = append(args, source, job.Destination+"/")

	cmd := exec.Command("rsync", args...)
	out, err := cmd.CombinedOutput()
	outStr := string(out)
	if len(outStr) > 2000 {
		outStr = outStr[:2000] + "\n... (truncated)"
	}

	return outStr, err
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", m, s)
}

// ── Restore ──

func restoreBackup(run BackupRun) (string, error) {
	if run.Type != "tar" || run.FilePath == "" {
		return "", fmt.Errorf("restore only supported for tar backups")
	}

	if _, err := os.Stat(run.FilePath); os.IsNotExist(err) {
		return "", fmt.Errorf("backup file not found: %s", run.FilePath)
	}

	// Detect compression
	var args []string
	if strings.HasSuffix(run.FilePath, ".tar.gz") || strings.HasSuffix(run.FilePath, ".tgz") {
		args = []string{"-xzf", run.FilePath, "-C", run.Source}
	} else {
		args = []string{"-xf", run.FilePath, "-C", run.Source}
	}

	cmd := exec.Command("tar", args...)
	out, err := cmd.CombinedOutput()
	outStr := string(out)
	if len(outStr) > 2000 {
		outStr = outStr[:2000] + "\n... (truncated)"
	}
	return outStr, err
}
