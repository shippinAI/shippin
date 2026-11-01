package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var commonLogFiles = []string{
	"/var/log/syslog",
	"/var/log/auth.log",
	"/var/log/kern.log",
	"/var/log/dpkg.log",
	"/var/log/apt/history.log",
	"/var/log/ufw.log",
	"/var/log/faillog",
	"/var/log/nginx/access.log",
	"/var/log/nginx/error.log",
	"/var/log/apache2/access.log",
	"/var/log/apache2/error.log",
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	// Discover available log files
	available := discoverLogFiles()

	// Get list of systemd units for the journal dropdown
	units := getJournalUnits()

	hd := buildHeaderData(r)
	hd.PageTitle = "Logs"
	data := struct {
		HeaderData
		LogFiles []string
		Units    []string
	}{
		HeaderData: hd,
		LogFiles:   available,
		Units:      units,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "logs.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPILogs(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	lines := r.URL.Query().Get("lines")
	unit := r.URL.Query().Get("unit")
	filter := r.URL.Query().Get("filter")
	priority := r.URL.Query().Get("priority")

	numLines := 100
	if n, err := strconv.Atoi(lines); err == nil && n > 0 && n <= 5000 {
		numLines = n
	}

	var output string
	var err error

	switch source {
	case "journal":
		output, err = fetchJournalLogs(r, unit, numLines, priority, filter)
	case "file":
		file := r.URL.Query().Get("file")
		output, err = fetchFileLogs(r, file, numLines, filter)
	default:
		output, err = fetchJournalLogs(r, "", numLines, priority, filter)
	}

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"logs":    output,
	})
}

func fetchJournalLogs(r *http.Request, unit string, lines int, priority, filter string) (string, error) {
	args := []string{"journalctl", "--no-pager", "-n", strconv.Itoa(lines), "--output=short-iso"}

	if unit != "" {
		args = append(args, "-u", unit)
	}
	if priority != "" {
		args = append(args, "-p", priority)
	}

	if isSudoEnabled(r) {
		_, password, ok := getSessionPassword(r)
		if !ok {
			return "", fmt.Errorf("session expired")
		}
		out, err := sudoExecCommand(password, args[0], args[1:]...)
		if err != nil {
			return filterOutput(out, filter), err
		}
		return filterOutput(out, filter), nil
	}

	out, err := execCommand(args[0], args[1:]...)
	if err != nil {
		return filterOutput(out, filter), err
	}
	return filterOutput(out, filter), nil
}

func fetchFileLogs(r *http.Request, file string, lines int, filter string) (string, error) {
	// Validate file path — only allow /var/log paths
	clean := filepath.Clean(file)
	if !strings.HasPrefix(clean, "/var/log/") {
		return "", fmt.Errorf("access denied: only /var/log/ files are allowed")
	}

	args := []string{"tail", "-n", strconv.Itoa(lines), clean}

	if isSudoEnabled(r) {
		_, password, ok := getSessionPassword(r)
		if !ok {
			return "", fmt.Errorf("session expired")
		}
		out, err := sudoExecCommand(password, args[0], args[1:]...)
		if err != nil {
			return filterOutput(out, filter), err
		}
		return filterOutput(out, filter), nil
	}

	out, err := execCommand(args[0], args[1:]...)
	if err != nil {
		return filterOutput(out, filter), err
	}
	return filterOutput(out, filter), nil
}

func filterOutput(output, filter string) string {
	if filter == "" {
		return output
	}
	filter = strings.ToLower(filter)
	var filtered []string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(strings.ToLower(line), filter) {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}

func discoverLogFiles() []string {
	var available []string
	for _, f := range commonLogFiles {
		if _, err := os.Stat(f); err == nil {
			available = append(available, f)
		}
	}
	// Also scan /var/log for additional .log files
	entries, _ := os.ReadDir("/var/log")
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
			path := "/var/log/" + e.Name()
			found := false
			for _, a := range available {
				if a == path {
					found = true
					break
				}
			}
			if !found {
				available = append(available, path)
			}
		}
	}
	return available
}

func getJournalUnits() []string {
	out, err := execCommand("systemctl", "list-units", "--type=service", "--no-pager", "--no-legend", "--plain")
	if err != nil {
		return nil
	}
	var units []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			units = append(units, fields[0])
		}
	}
	return units
}
