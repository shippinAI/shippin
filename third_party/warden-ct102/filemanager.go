package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type FileEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"modTime"`
	Owner   string `json:"owner"`
}

func handleFileManager(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		dir = "/"
	}
	dir = filepath.Clean(dir)

	// Check if editing a file
	editPath := r.URL.Query().Get("edit")
	var editContent string
	var editName string
	if editPath != "" {
		editPath = filepath.Clean(editPath)
		content, err := readFileContent(r, editPath)
		if err != nil {
			editContent = "Error reading file: " + err.Error()
		} else {
			editContent = content
		}
		editName = editPath
	}

	entries, err := listDirectory(r, dir)
	var errMsg string
	if err != nil {
		errMsg = "Cannot read directory: " + err.Error()
	}

	hd := buildHeaderData(r)
	hd.PageTitle = "Files"
	data := struct {
		HeaderData
		Message     string
		Dir         string
		ParentDir   string
		Entries     []FileEntry
		EditPath    string
		EditContent string
		Error       string
	}{
		HeaderData:  hd,
		Message:     r.URL.Query().Get("msg"),
		Dir:         dir,
		ParentDir:   filepath.Dir(dir),
		Entries:     entries,
		EditPath:    editName,
		EditContent: editContent,
		Error:       errMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "filemanager.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func listDirectory(r *http.Request, dir string) ([]FileEntry, error) {
	var output string
	var err error

	if isSudoEnabled(r) {
		_, password, ok := getSessionPassword(r)
		if !ok {
			return nil, fmt.Errorf("session expired")
		}
		// Use ls -la to get file listing with sudo
		output, err = sudoExecCommand(password, "ls", "-la", "--time-style=long-iso", dir)
	} else {
		output, err = execCommand("ls", "-la", "--time-style=long-iso", dir)
	}
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(output))
	}

	var entries []FileEntry
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "total ") || line == "" {
			continue
		}
		entry := parseLsLine(line)
		if entry.Name == "." || entry.Name == "" {
			continue
		}
		entries = append(entries, entry)
	}

	// Sort: directories first, then by name
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	return entries, nil
}

func parseLsLine(line string) FileEntry {
	// ls -la --time-style=long-iso format:
	// drwxr-xr-x 2 user group 4096 2024-01-15 10:30 dirname
	fields := strings.Fields(line)
	if len(fields) < 8 {
		return FileEntry{}
	}

	mode := fields[0]
	owner := fields[2]
	var size int64
	fmt.Sscanf(fields[4], "%d", &size)

	// Date is fields[5] + fields[6]
	modTime := fields[5] + " " + fields[6]

	// Name is everything after the date (handles spaces in filenames)
	nameStart := 0
	fieldCount := 0
	for i, ch := range line {
		if ch == ' ' {
			// Skip consecutive spaces
			if i+1 < len(line) && line[i+1] != ' ' {
				fieldCount++
			}
			if fieldCount == 7 {
				nameStart = i + 1
				break
			}
		}
	}
	name := line[nameStart:]

	// Handle symlinks: name -> target
	if strings.Contains(name, " -> ") {
		parts := strings.SplitN(name, " -> ", 2)
		name = parts[0]
	}

	isDir := mode[0] == 'd'

	return FileEntry{
		Name:    name,
		IsDir:   isDir,
		Size:    size,
		Mode:    mode,
		ModTime: modTime,
		Owner:   owner,
	}
}

func readFileContent(r *http.Request, path string) (string, error) {
	if isSudoEnabled(r) {
		_, password, ok := getSessionPassword(r)
		if !ok {
			return "", fmt.Errorf("session expired")
		}
		return sudoExecCommand(password, "cat", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func handleFileAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	action := r.URL.Query().Get("action")
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		dir = "/"
	}

	switch action {
	case "save":
		handleFileSave(w, r, dir)
	case "delete":
		handleFileDelete(w, r, dir)
	case "mkdir":
		handleFileMkdir(w, r, dir)
	case "create":
		handleFileCreate(w, r, dir)
	case "rename":
		handleFileRename(w, r, dir)
	case "upload":
		handleFileUpload(w, r, dir)
	default:
		respondFile(w, r, false, "Unknown action", dir)
	}
}

func handleFileSave(w http.ResponseWriter, r *http.Request, dir string) {
	r.ParseForm()
	path := r.FormValue("path")
	content := r.FormValue("content")

	if path == "" {
		respondFile(w, r, false, "No file path specified", dir)
		return
	}
	path = filepath.Clean(path)

	if isSudoEnabled(r) {
		_, password, ok := getSessionPassword(r)
		if !ok {
			respondFile(w, r, false, "Session expired", dir)
			return
		}
		// Write via sudo tee
		_, err := sudoExecCommandStdin(password, content, "tee", path)
		if err != nil {
			respondFile(w, r, false, "Failed to save: "+err.Error(), dir)
			return
		}
	} else {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			respondFile(w, r, false, "Failed to save: "+err.Error(), dir)
			return
		}
	}

	auditFromRequest(r, "File saved", path, "file")
	respondFile(w, r, true, "File saved successfully", filepath.Dir(path))
}

func handleFileDelete(w http.ResponseWriter, r *http.Request, dir string) {
	r.ParseForm()
	target := r.FormValue("target")
	if target == "" {
		respondFile(w, r, false, "No target specified", dir)
		return
	}
	target = filepath.Clean(target)

	// Safety: don't allow deleting root or critical paths
	if target == "/" || target == "/etc" || target == "/usr" || target == "/bin" || target == "/var" || target == "/home" {
		respondFile(w, r, false, "Cannot delete critical system path", dir)
		return
	}

	var err error
	if isSudoEnabled(r) {
		_, password, ok := getSessionPassword(r)
		if !ok {
			respondFile(w, r, false, "Session expired", dir)
			return
		}
		out, e := sudoExecCommand(password, "rm", "-rf", target)
		if e != nil {
			respondFile(w, r, false, "Delete failed: "+strings.TrimSpace(out), dir)
			return
		}
	} else {
		err = os.RemoveAll(target)
		if err != nil {
			respondFile(w, r, false, "Delete failed: "+err.Error(), dir)
			return
		}
	}

	auditFromRequest(r, "File deleted", target, "file")
	respondFile(w, r, true, "Deleted successfully", dir)
}

func handleFileMkdir(w http.ResponseWriter, r *http.Request, dir string) {
	r.ParseForm()
	name := r.FormValue("name")
	if name == "" {
		respondFile(w, r, false, "No directory name specified", dir)
		return
	}
	// Sanitize: no path separators in name
	if strings.ContainsAny(name, "/\\") {
		respondFile(w, r, false, "Invalid directory name", dir)
		return
	}

	target := filepath.Join(dir, name)

	if isSudoEnabled(r) {
		_, password, ok := getSessionPassword(r)
		if !ok {
			respondFile(w, r, false, "Session expired", dir)
			return
		}
		out, err := sudoExecCommand(password, "mkdir", "-p", target)
		if err != nil {
			respondFile(w, r, false, "Failed: "+strings.TrimSpace(out), dir)
			return
		}
	} else {
		if err := os.MkdirAll(target, 0755); err != nil {
			respondFile(w, r, false, "Failed: "+err.Error(), dir)
			return
		}
	}

	auditFromRequest(r, "Directory created", target, "file")
	respondFile(w, r, true, "Directory created", dir)
}

func handleFileCreate(w http.ResponseWriter, r *http.Request, dir string) {
	r.ParseForm()
	name := r.FormValue("name")
	if name == "" {
		respondFile(w, r, false, "No file name specified", dir)
		return
	}
	if strings.ContainsAny(name, "/\\") {
		respondFile(w, r, false, "Invalid file name", dir)
		return
	}

	target := filepath.Join(dir, name)

	if isSudoEnabled(r) {
		_, password, ok := getSessionPassword(r)
		if !ok {
			respondFile(w, r, false, "Session expired", dir)
			return
		}
		_, err := sudoExecCommand(password, "touch", target)
		if err != nil {
			respondFile(w, r, false, "Failed to create file", dir)
			return
		}
	} else {
		f, err := os.Create(target)
		if err != nil {
			respondFile(w, r, false, "Failed: "+err.Error(), dir)
			return
		}
		f.Close()
	}

	auditFromRequest(r, "File created", target, "file")
	respondFile(w, r, true, "File created", dir)
}

func handleFileRename(w http.ResponseWriter, r *http.Request, dir string) {
	r.ParseForm()
	oldPath := r.FormValue("old")
	newName := r.FormValue("new")
	if oldPath == "" || newName == "" {
		respondFile(w, r, false, "Missing parameters", dir)
		return
	}
	if strings.ContainsAny(newName, "/\\") {
		respondFile(w, r, false, "Invalid name", dir)
		return
	}

	oldPath = filepath.Clean(oldPath)
	newPath := filepath.Join(filepath.Dir(oldPath), newName)

	if isSudoEnabled(r) {
		_, password, ok := getSessionPassword(r)
		if !ok {
			respondFile(w, r, false, "Session expired", dir)
			return
		}
		out, err := sudoExecCommand(password, "mv", oldPath, newPath)
		if err != nil {
			respondFile(w, r, false, "Rename failed: "+strings.TrimSpace(out), dir)
			return
		}
	} else {
		if err := os.Rename(oldPath, newPath); err != nil {
			respondFile(w, r, false, "Rename failed: "+err.Error(), dir)
			return
		}
	}

	auditFromRequest(r, "File renamed", oldPath+" -> "+newName, "file")
	respondFile(w, r, true, "Renamed successfully", dir)
}

func handleFileUpload(w http.ResponseWriter, r *http.Request, dir string) {
	// Limit upload size to 100MB
	r.Body = http.MaxBytesReader(w, r.Body, 100<<20)

	file, header, err := r.FormFile("file")
	if err != nil {
		respondFile(w, r, false, "Upload failed: "+err.Error(), dir)
		return
	}
	defer file.Close()

	name := filepath.Base(header.Filename)
	target := filepath.Join(dir, name)

	if isSudoEnabled(r) {
		_, password, ok := getSessionPassword(r)
		if !ok {
			respondFile(w, r, false, "Session expired", dir)
			return
		}
		// Read upload into memory, write via sudo tee
		data, err := io.ReadAll(file)
		if err != nil {
			respondFile(w, r, false, "Upload read failed: "+err.Error(), dir)
			return
		}
		_, err = sudoExecCommandStdin(password, string(data), "tee", target)
		if err != nil {
			respondFile(w, r, false, "Upload write failed: "+err.Error(), dir)
			return
		}
	} else {
		dst, err := os.Create(target)
		if err != nil {
			respondFile(w, r, false, "Failed to create file: "+err.Error(), dir)
			return
		}
		defer dst.Close()
		if _, err := io.Copy(dst, file); err != nil {
			respondFile(w, r, false, "Upload failed: "+err.Error(), dir)
			return
		}
	}

	auditFromRequest(r, "File uploaded", target, "file")
	respondFile(w, r, true, "Uploaded "+name, dir)
}

func handleFileDownload(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "No path specified", http.StatusBadRequest)
		return
	}
	path = filepath.Clean(path)

	// Get file info
	info, err := os.Stat(path)
	if err != nil {
		// Try with sudo
		if isSudoEnabled(r) {
			_, password, ok := getSessionPassword(r)
			if !ok {
				http.Error(w, "Session expired", http.StatusUnauthorized)
				return
			}
			content, err := sudoExecCommand(password, "cat", path)
			if err != nil {
				http.Error(w, "Cannot read file", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(path)+"\"")
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte(content))
			return
		}
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	if info.IsDir() {
		http.Error(w, "Cannot download directory", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(path)+"\"")
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, path)
}

func respondFile(w http.ResponseWriter, r *http.Request, success bool, msg, dir string) {
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": success,
			"message": msg,
		})
		return
	}
	http.Redirect(w, r, "/files?dir="+dir+"&msg="+msg, http.StatusSeeOther)
}

// sudoExecCommandStdin runs a command with sudo, providing both password and custom stdin.
func sudoExecCommandStdin(password, stdinData, name string, args ...string) (string, error) {
	if os.Getuid() == 0 {
		cmd := exec.Command(name, args...)
		cmd.Stdin = strings.NewReader(stdinData)
		out, err := cmd.Output()
		return string(out), err
	}

	// Use sudo -S with password on stdin, then pipe data via a shell
	sudoArgs := append([]string{"-S", "--", name}, args...)
	cmd := exec.Command("sudo", sudoArgs...)
	// Provide password + newline + data on stdin
	cmd.Stdin = bytes.NewBufferString(password + "\n" + stdinData)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stderr.String(), err
	}
	return stdout.String(), nil
}

// formatFileSize returns a human-readable file size
func formatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	} else if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
}

// isTextFile checks if a file is likely a text file based on extension
func isTextFile(name string) bool {
	textExts := map[string]bool{
		".txt": true, ".md": true, ".log": true, ".conf": true, ".cfg": true,
		".ini": true, ".yaml": true, ".yml": true, ".json": true, ".xml": true,
		".html": true, ".htm": true, ".css": true, ".js": true, ".ts": true,
		".go": true, ".py": true, ".rb": true, ".sh": true, ".bash": true,
		".zsh": true, ".fish": true, ".pl": true, ".rs": true, ".c": true,
		".h": true, ".cpp": true, ".java": true, ".php": true, ".sql": true,
		".env": true, ".toml": true, ".service": true, ".timer": true,
		".socket": true, ".mount": true, ".rules": true, ".csv": true,
		".gitignore": true, ".dockerignore": true, ".editorconfig": true,
		"": true, // extensionless files (Makefile, Dockerfile, etc.)
	}
	ext := strings.ToLower(filepath.Ext(name))
	return textExts[ext]
}

