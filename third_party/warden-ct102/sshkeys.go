package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

type SSHKey struct {
	Line        int    `json:"line"`
	Type        string `json:"type"`
	Fingerprint string `json:"fingerprint"`
	Comment     string `json:"comment"`
	Raw         string `json:"raw"`
}

// Valid SSH key types
var validKeyTypes = map[string]bool{
	"ssh-rsa":                          true,
	"ssh-ed25519":                      true,
	"ssh-dss":                          true,
	"ecdsa-sha2-nistp256":              true,
	"ecdsa-sha2-nistp384":              true,
	"ecdsa-sha2-nistp521":              true,
	"sk-ssh-ed25519@openssh.com":       true,
	"sk-ecdsa-sha2-nistp256@openssh.com": true,
}

// usernameRe validates usernames for path traversal prevention
var usernameRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

func handleSSHKeys(w http.ResponseWriter, r *http.Request) {
	username, _ := getSession(r)

	targetUser := r.URL.Query().Get("user")
	if targetUser == "" {
		targetUser = username
	}

	// Validate target user
	if !usernameRe.MatchString(targetUser) {
		http.Error(w, "Invalid username", http.StatusBadRequest)
		return
	}

	keys := listSSHKeys(r, targetUser)
	users := listSystemUsers()

	hd := buildHeaderData(r)
	hd.PageTitle = "SSH Keys"
	data := struct {
		HeaderData
		Keys       []SSHKey
		TargetUser string
		Users      []string
	}{
		HeaderData: hd,
		Keys:       keys,
		TargetUser: targetUser,
		Users:      users,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "sshkeys.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPISSHKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		username, _ := getSession(r)
		targetUser := r.URL.Query().Get("user")
		if targetUser == "" {
			targetUser = username
		}
		if !usernameRe.MatchString(targetUser) {
			respondJSON(w, false, "Invalid username")
			return
		}
		keys := listSSHKeys(r, targetUser)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"keys":    keys,
			"user":    targetUser,
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
		handleSSHKeyAdd(w, r)
	case "delete":
		handleSSHKeyDelete(w, r)
	default:
		respondJSON(w, false, "Invalid action")
	}
}

func handleSSHKeyAdd(w http.ResponseWriter, r *http.Request) {
	username, _ := getSession(r)
	targetUser := r.FormValue("user")
	keyData := strings.TrimSpace(r.FormValue("key"))

	if targetUser == "" {
		targetUser = username
	}
	if !usernameRe.MatchString(targetUser) {
		respondJSON(w, false, "Invalid username")
		return
	}
	if keyData == "" {
		respondJSON(w, false, "SSH key is required")
		return
	}

	// Validate key format
	if err := validateSSHKey(keyData); err != nil {
		respondJSON(w, false, "Invalid SSH key: "+err.Error())
		return
	}

	// Need sudo to write to other users' home dirs
	needsSudo := targetUser != username

	authKeysPath := getAuthorizedKeysPath(targetUser)
	if authKeysPath == "" {
		respondJSON(w, false, "Cannot determine home directory for user "+targetUser)
		return
	}

	// Ensure .ssh directory exists
	sshDir := filepath.Dir(authKeysPath)
	if needsSudo && isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		// Create .ssh dir
		sudoExecCommand(pass, "mkdir", "-p", sshDir)
		sudoExecCommand(pass, "chmod", "700", sshDir)
		sudoExecCommand(pass, "chown", targetUser+":"+targetUser, sshDir)

		// Append key
		// Use tee -a to append since we can't redirect with sudo
		out, err := sudoExecCommand(pass, "bash", "-c", "echo "+shellQuote(keyData)+" >> "+shellQuote(authKeysPath))
		if err != nil {
			respondJSON(w, false, "Failed to add key: "+strings.TrimSpace(out))
			return
		}
		sudoExecCommand(pass, "chmod", "600", authKeysPath)
		sudoExecCommand(pass, "chown", targetUser+":"+targetUser, authKeysPath)
	} else if !needsSudo || targetUser == username {
		// Writing to own authorized_keys
		if err := os.MkdirAll(sshDir, 0700); err != nil {
			respondJSON(w, false, "Failed to create .ssh directory: "+err.Error())
			return
		}

		f, err := os.OpenFile(authKeysPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			respondJSON(w, false, "Failed to open authorized_keys: "+err.Error())
			return
		}
		defer f.Close()

		if _, err := f.WriteString(keyData + "\n"); err != nil {
			respondJSON(w, false, "Failed to write key: "+err.Error())
			return
		}
	} else {
		respondJSON(w, false, "Sudo must be enabled to manage other users' SSH keys")
		return
	}

	auditFromRequest(r, "SSH key added", "User: "+targetUser, "sshkey")
	respondJSON(w, true, "SSH key added for "+targetUser)
}

func handleSSHKeyDelete(w http.ResponseWriter, r *http.Request) {
	username, _ := getSession(r)
	targetUser := r.FormValue("user")
	lineStr := r.FormValue("line")

	if targetUser == "" {
		targetUser = username
	}
	if !usernameRe.MatchString(targetUser) {
		respondJSON(w, false, "Invalid username")
		return
	}

	lineNum := 0
	fmt.Sscanf(lineStr, "%d", &lineNum)
	if lineNum < 1 {
		respondJSON(w, false, "Invalid line number")
		return
	}

	needsSudo := targetUser != username
	authKeysPath := getAuthorizedKeysPath(targetUser)
	if authKeysPath == "" {
		respondJSON(w, false, "Cannot determine home directory for user "+targetUser)
		return
	}

	// Read current file
	var content string
	if needsSudo && isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err := sudoExecCommand(pass, "cat", authKeysPath)
		if err != nil {
			respondJSON(w, false, "Failed to read authorized_keys")
			return
		}
		content = out
	} else if !needsSudo {
		data, err := os.ReadFile(authKeysPath)
		if err != nil {
			respondJSON(w, false, "Failed to read authorized_keys: "+err.Error())
			return
		}
		content = string(data)
	} else {
		respondJSON(w, false, "Sudo must be enabled to manage other users' SSH keys")
		return
	}

	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if lineNum > len(lines) {
		respondJSON(w, false, "Line number out of range")
		return
	}

	// Remove the line (1-indexed)
	newLines := append(lines[:lineNum-1], lines[lineNum:]...)
	newContent := strings.Join(newLines, "\n")
	if len(newLines) > 0 {
		newContent += "\n"
	}

	// Write back
	if needsSudo && isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err := sudoExecCommand(pass, "bash", "-c", "echo "+shellQuote(strings.TrimRight(newContent, "\n"))+" > "+shellQuote(authKeysPath))
		if err != nil {
			respondJSON(w, false, "Failed to write authorized_keys: "+strings.TrimSpace(out))
			return
		}
	} else {
		if err := os.WriteFile(authKeysPath, []byte(newContent), 0600); err != nil {
			respondJSON(w, false, "Failed to write authorized_keys: "+err.Error())
			return
		}
	}

	auditFromRequest(r, "SSH key deleted", "User: "+targetUser+", line "+lineStr, "sshkey")
	respondJSON(w, true, "SSH key removed")
}

func listSSHKeys(r *http.Request, targetUser string) []SSHKey {
	username, _ := getSession(r)
	authKeysPath := getAuthorizedKeysPath(targetUser)
	if authKeysPath == "" {
		return nil
	}

	var content string
	needsSudo := targetUser != username

	if needsSudo && isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			return nil
		}
		out, _ := sudoExecCommand(pass, "cat", authKeysPath)
		content = out
	} else {
		data, err := os.ReadFile(authKeysPath)
		if err != nil {
			return nil
		}
		content = string(data)
	}

	var keys []SSHKey
	for i, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key := parseSSHKey(line, i+1)
		if key != nil {
			keys = append(keys, *key)
		}
	}
	return keys
}

func parseSSHKey(line string, lineNum int) *SSHKey {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return nil
	}

	// Handle optional options prefix (e.g., "no-pty,command=..." before key type)
	keyTypeIdx := 0
	if !validKeyTypes[parts[0]] && len(parts) >= 3 {
		keyTypeIdx = 1
	}

	if keyTypeIdx >= len(parts) || !validKeyTypes[parts[keyTypeIdx]] {
		// Unknown format — still show it
		return &SSHKey{
			Line:    lineNum,
			Type:    "unknown",
			Comment: line[:min(50, len(line))],
			Raw:     line,
		}
	}

	keyType := parts[keyTypeIdx]
	keyData := ""
	if keyTypeIdx+1 < len(parts) {
		keyData = parts[keyTypeIdx+1]
	}

	comment := ""
	if keyTypeIdx+2 < len(parts) {
		comment = strings.Join(parts[keyTypeIdx+2:], " ")
	}

	fingerprint := computeFingerprint(keyData)

	return &SSHKey{
		Line:        lineNum,
		Type:        keyType,
		Fingerprint: fingerprint,
		Comment:     comment,
		Raw:         line,
	}
}

func computeFingerprint(keyData string) string {
	decoded, err := base64.StdEncoding.DecodeString(keyData)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(decoded)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(hash[:])
}

func validateSSHKey(key string) error {
	parts := strings.Fields(key)
	if len(parts) < 2 {
		return fmt.Errorf("key must have at least a type and data component")
	}

	// Find the key type (may have options prefix)
	keyTypeIdx := 0
	if !validKeyTypes[parts[0]] && len(parts) >= 3 {
		keyTypeIdx = 1
	}

	if keyTypeIdx >= len(parts) || !validKeyTypes[parts[keyTypeIdx]] {
		return fmt.Errorf("unsupported key type %q (supported: ssh-rsa, ssh-ed25519, ecdsa-*)", parts[keyTypeIdx])
	}

	if keyTypeIdx+1 >= len(parts) {
		return fmt.Errorf("missing key data")
	}

	// Validate base64 data
	keyData := parts[keyTypeIdx+1]
	if _, err := base64.StdEncoding.DecodeString(keyData); err != nil {
		return fmt.Errorf("invalid base64 key data")
	}

	return nil
}

func getAuthorizedKeysPath(username string) string {
	u, err := user.Lookup(username)
	if err != nil {
		return ""
	}
	return filepath.Join(u.HomeDir, ".ssh", "authorized_keys")
}

func listSystemUsers() []string {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil
	}
	var users []string
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		shell := parts[6]
		// Only include users with a real shell
		if shell == "/usr/sbin/nologin" || shell == "/bin/false" || shell == "/sbin/nologin" {
			continue
		}
		users = append(users, parts[0])
	}
	return users
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
