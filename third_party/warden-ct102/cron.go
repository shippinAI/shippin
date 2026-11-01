package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
)

type CronJob struct {
	Index    int    `json:"index"`
	Minute   string `json:"minute"`
	Hour     string `json:"hour"`
	Day      string `json:"day"`
	Month    string `json:"month"`
	Weekday  string `json:"weekday"`
	Command  string `json:"command"`
	Schedule string `json:"schedule"`
	Raw      string `json:"raw"`
}

func handleCron(w http.ResponseWriter, r *http.Request) {
	jobs := listCronJobs(r)

	hd := buildHeaderData(r)
	hd.PageTitle = "Cron Jobs"
	data := struct {
		HeaderData
		Jobs []CronJob
	}{
		HeaderData: hd,
		Jobs:       jobs,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "cron.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPICron(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		jobs := listCronJobs(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"jobs":    jobs,
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
		handleCronAdd(w, r)
	case "delete":
		handleCronDelete(w, r)
	case "edit":
		handleCronEdit(w, r)
	default:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Invalid action",
		})
	}
}

func handleCronAdd(w http.ResponseWriter, r *http.Request) {
	minute := r.FormValue("minute")
	hour := r.FormValue("hour")
	day := r.FormValue("day")
	month := r.FormValue("month")
	weekday := r.FormValue("weekday")
	command := r.FormValue("command")

	if command == "" {
		respondJSON(w, false, "Command is required")
		return
	}

	// Default to * if empty
	if minute == "" {
		minute = "*"
	}
	if hour == "" {
		hour = "*"
	}
	if day == "" {
		day = "*"
	}
	if month == "" {
		month = "*"
	}
	if weekday == "" {
		weekday = "*"
	}

	newLine := fmt.Sprintf("%s %s %s %s %s %s", minute, hour, day, month, weekday, command)

	existing := getCrontab(r)
	var updated string
	if existing != "" {
		updated = existing + "\n" + newLine + "\n"
	} else {
		updated = newLine + "\n"
	}

	if err := setCrontab(r, updated); err != nil {
		respondJSON(w, false, "Failed to add cron job: "+err.Error())
		return
	}

	auditFromRequest(r, "Cron job added", newLine, "cron")
	respondJSON(w, true, "Cron job added")
}

func handleCronDelete(w http.ResponseWriter, r *http.Request) {
	raw := r.FormValue("raw")
	if raw == "" {
		respondJSON(w, false, "No job specified")
		return
	}

	existing := getCrontab(r)
	lines := strings.Split(existing, "\n")
	var newLines []string
	removed := false
	for _, line := range lines {
		if !removed && strings.TrimSpace(line) == strings.TrimSpace(raw) {
			removed = true
			continue
		}
		newLines = append(newLines, line)
	}

	if !removed {
		respondJSON(w, false, "Job not found")
		return
	}

	if err := setCrontab(r, strings.Join(newLines, "\n")); err != nil {
		respondJSON(w, false, "Failed to delete cron job: "+err.Error())
		return
	}

	auditFromRequest(r, "Cron job deleted", raw, "cron")
	respondJSON(w, true, "Cron job deleted")
}

func handleCronEdit(w http.ResponseWriter, r *http.Request) {
	oldRaw := r.FormValue("old_raw")
	minute := r.FormValue("minute")
	hour := r.FormValue("hour")
	day := r.FormValue("day")
	month := r.FormValue("month")
	weekday := r.FormValue("weekday")
	command := r.FormValue("command")

	if oldRaw == "" || command == "" {
		respondJSON(w, false, "Missing required fields")
		return
	}

	if minute == "" {
		minute = "*"
	}
	if hour == "" {
		hour = "*"
	}
	if day == "" {
		day = "*"
	}
	if month == "" {
		month = "*"
	}
	if weekday == "" {
		weekday = "*"
	}

	newLine := fmt.Sprintf("%s %s %s %s %s %s", minute, hour, day, month, weekday, command)

	existing := getCrontab(r)
	lines := strings.Split(existing, "\n")
	replaced := false
	for i, line := range lines {
		if !replaced && strings.TrimSpace(line) == strings.TrimSpace(oldRaw) {
			lines[i] = newLine
			replaced = true
		}
	}

	if !replaced {
		respondJSON(w, false, "Original job not found")
		return
	}

	if err := setCrontab(r, strings.Join(lines, "\n")); err != nil {
		respondJSON(w, false, "Failed to edit cron job: "+err.Error())
		return
	}

	auditFromRequest(r, "Cron job edited", newLine, "cron")
	respondJSON(w, true, "Cron job updated")
}

func respondJSON(w http.ResponseWriter, success bool, message string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": success,
		"message": message,
	})
}

func listCronJobs(r *http.Request) []CronJob {
	raw := getCrontab(r)
	var jobs []CronJob
	idx := 0
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		job := parseCronLine(line, idx)
		if job != nil {
			jobs = append(jobs, *job)
			idx++
		}
	}
	return jobs
}

func parseCronLine(line string, index int) *CronJob {
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return nil
	}

	schedule := fmt.Sprintf("%s %s %s %s %s", fields[0], fields[1], fields[2], fields[3], fields[4])

	return &CronJob{
		Index:    index,
		Minute:   fields[0],
		Hour:     fields[1],
		Day:      fields[2],
		Month:    fields[3],
		Weekday:  fields[4],
		Command:  strings.Join(fields[5:], " "),
		Schedule: schedule,
		Raw:      line,
	}
}

func getCrontab(r *http.Request) string {
	if isSudoEnabled(r) {
		_, password, ok := getSessionPassword(r)
		if !ok {
			return ""
		}
		out, _ := sudoExecCommand(password, "crontab", "-l")
		return out
	}
	out, _ := execCommand("crontab", "-l")
	return out
}

func setCrontab(r *http.Request, content string) error {
	// Ensure content ends with newline
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	if isSudoEnabled(r) {
		_, password, ok := getSessionPassword(r)
		if !ok {
			return fmt.Errorf("session expired")
		}
		_, err := sudoExecCommandStdin(password, content, "crontab", "-")
		return err
	}

	// Use stdin to pipe content to crontab
	_, err := execCommandStdin(content, "crontab", "-")
	return err
}

func execCommandStdin(stdin, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
