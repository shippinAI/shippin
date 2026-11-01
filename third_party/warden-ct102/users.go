package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type User struct {
	Username string `json:"username"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
	Comment  string `json:"comment"`
	Home     string `json:"home"`
	Shell    string `json:"shell"`
	Groups   string `json:"groups"`
	Locked   bool   `json:"locked"`
}

type Group struct {
	Name    string   `json:"name"`
	GID     int      `json:"gid"`
	Members []string `json:"members"`
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	users := listUsers(r)
	groups := listGroups(r)
	shells := listShells()

	hd := buildHeaderData(r)
	hd.PageTitle = "Users"
	data := struct {
		HeaderData
		Users  []User
		Groups []Group
		Shells []string
	}{
		HeaderData: hd,
		Users:      users,
		Groups:     groups,
		Shells:     shells,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "users.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		users := listUsers(r)
		groups := listGroups(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"users":   users,
			"groups":  groups,
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
	case "add_user":
		handleAddUser(w, r)
	case "delete_user":
		handleDeleteUser(w, r)
	case "modify_user":
		handleModifyUser(w, r)
	case "change_password":
		handleChangePassword(w, r)
	case "toggle_lock":
		handleToggleLock(w, r)
	case "add_group":
		handleAddGroup(w, r)
	case "delete_group":
		handleDeleteGroup(w, r)
	case "modify_group":
		handleModifyGroup(w, r)
	default:
		respondJSON(w, false, "Invalid action")
	}
}

// validUsername checks for a safe username pattern
var validUsername = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

func handleAddUser(w http.ResponseWriter, r *http.Request) {
	uname := r.FormValue("username")
	shell := r.FormValue("shell")
	home := r.FormValue("home")
	comment := r.FormValue("comment")
	password := r.FormValue("password")
	createHome := r.FormValue("create_home")

	if uname == "" {
		respondJSON(w, false, "Username is required")
		return
	}
	if !validUsername.MatchString(uname) {
		respondJSON(w, false, "Invalid username: must start with a-z or _, and contain only a-z, 0-9, _, -")
		return
	}

	args := []string{}
	if createHome == "on" || createHome == "true" {
		args = append(args, "-m")
	}
	if shell != "" {
		args = append(args, "-s", shell)
	}
	if home != "" {
		args = append(args, "-d", home)
	}
	if comment != "" {
		args = append(args, "-c", comment)
	}
	args = append(args, uname)

	var out string
	var err error
	if isSudoEnabled(r) {
		_, sudoPass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(sudoPass, "useradd", args...)
	} else {
		out, err = execCommand("useradd", args...)
	}

	if err != nil {
		respondJSON(w, false, "Failed to add user: "+strings.TrimSpace(out))
		return
	}

	// Set password if provided
	if password != "" {
		if err := setUserPassword(r, uname, password); err != nil {
			respondJSON(w, false, "User created but failed to set password: "+err.Error())
			return
		}
	}

	auditFromRequest(r, "User created", uname, "user")
	respondJSON(w, true, "User '"+uname+"' created")
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	uname := r.FormValue("username")
	removeHome := r.FormValue("remove_home")

	if uname == "" {
		respondJSON(w, false, "Username is required")
		return
	}
	if !validUsername.MatchString(uname) {
		respondJSON(w, false, "Invalid username")
		return
	}

	args := []string{}
	if removeHome == "on" || removeHome == "true" {
		args = append(args, "-r")
	}
	args = append(args, uname)

	var out string
	var err error
	if isSudoEnabled(r) {
		_, sudoPass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(sudoPass, "userdel", args...)
	} else {
		out, err = execCommand("userdel", args...)
	}

	if err != nil {
		respondJSON(w, false, "Failed to delete user: "+strings.TrimSpace(out))
		return
	}

	auditFromRequest(r, "User deleted", uname, "user")
	respondJSON(w, true, "User '"+uname+"' deleted")
}

func handleModifyUser(w http.ResponseWriter, r *http.Request) {
	uname := r.FormValue("username")
	shell := r.FormValue("shell")
	comment := r.FormValue("comment")
	groups := r.FormValue("groups")

	if uname == "" {
		respondJSON(w, false, "Username is required")
		return
	}
	if !validUsername.MatchString(uname) {
		respondJSON(w, false, "Invalid username")
		return
	}

	args := []string{}
	if shell != "" {
		args = append(args, "-s", shell)
	}
	if comment != "" {
		args = append(args, "-c", comment)
	}
	// Set supplementary groups (replaces existing)
	if groups != "" {
		args = append(args, "-G", groups)
	}
	args = append(args, uname)

	if len(args) == 1 {
		respondJSON(w, false, "No changes specified")
		return
	}

	var out string
	var err error
	if isSudoEnabled(r) {
		_, sudoPass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(sudoPass, "usermod", args...)
	} else {
		out, err = execCommand("usermod", args...)
	}

	if err != nil {
		respondJSON(w, false, "Failed to modify user: "+strings.TrimSpace(out))
		return
	}

	auditFromRequest(r, "User modified", uname, "user")
	respondJSON(w, true, "User '"+uname+"' updated")
}

func handleChangePassword(w http.ResponseWriter, r *http.Request) {
	uname := r.FormValue("username")
	password := r.FormValue("password")

	if uname == "" || password == "" {
		respondJSON(w, false, "Username and password are required")
		return
	}
	if !validUsername.MatchString(uname) {
		respondJSON(w, false, "Invalid username")
		return
	}

	if err := setUserPassword(r, uname, password); err != nil {
		respondJSON(w, false, "Failed to change password: "+err.Error())
		return
	}

	auditFromRequest(r, "Password changed", uname, "user")
	respondJSON(w, true, "Password changed for '"+uname+"'")
}

func handleToggleLock(w http.ResponseWriter, r *http.Request) {
	uname := r.FormValue("username")
	locked := r.FormValue("locked")

	if uname == "" {
		respondJSON(w, false, "Username is required")
		return
	}
	if !validUsername.MatchString(uname) {
		respondJSON(w, false, "Invalid username")
		return
	}

	flag := "-L" // lock
	action := "locked"
	if locked == "true" {
		flag = "-U" // unlock
		action = "unlocked"
	}

	var out string
	var err error
	if isSudoEnabled(r) {
		_, sudoPass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(sudoPass, "usermod", flag, uname)
	} else {
		out, err = execCommand("usermod", flag, uname)
	}

	if err != nil {
		respondJSON(w, false, "Failed to toggle lock: "+strings.TrimSpace(out))
		return
	}

	auditFromRequest(r, "User "+action, uname, "user")
	respondJSON(w, true, "User '"+uname+"' "+action)
}

func handleAddGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("groupname")
	gidStr := r.FormValue("gid")

	if name == "" {
		respondJSON(w, false, "Group name is required")
		return
	}
	if !validUsername.MatchString(name) {
		respondJSON(w, false, "Invalid group name")
		return
	}

	args := []string{}
	if gidStr != "" {
		if _, err := strconv.Atoi(gidStr); err != nil {
			respondJSON(w, false, "Invalid GID: must be a number")
			return
		}
		args = append(args, "-g", gidStr)
	}
	args = append(args, name)

	var out string
	var err error
	if isSudoEnabled(r) {
		_, sudoPass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(sudoPass, "groupadd", args...)
	} else {
		out, err = execCommand("groupadd", args...)
	}

	if err != nil {
		respondJSON(w, false, "Failed to add group: "+strings.TrimSpace(out))
		return
	}

	auditFromRequest(r, "Group created", name, "user")
	respondJSON(w, true, "Group '"+name+"' created")
}

func handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("groupname")

	if name == "" {
		respondJSON(w, false, "Group name is required")
		return
	}

	var out string
	var err error
	if isSudoEnabled(r) {
		_, sudoPass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(sudoPass, "groupdel", name)
	} else {
		out, err = execCommand("groupdel", name)
	}

	if err != nil {
		respondJSON(w, false, "Failed to delete group: "+strings.TrimSpace(out))
		return
	}

	auditFromRequest(r, "Group deleted", name, "user")
	respondJSON(w, true, "Group '"+name+"' deleted")
}

func handleModifyGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("groupname")
	members := r.FormValue("members")

	if name == "" {
		respondJSON(w, false, "Group name is required")
		return
	}

	// Use gpasswd to set member list
	var out string
	var err error
	if isSudoEnabled(r) {
		_, sudoPass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(sudoPass, "gpasswd", "-M", members, name)
	} else {
		out, err = execCommand("gpasswd", "-M", members, name)
	}

	if err != nil {
		respondJSON(w, false, "Failed to modify group: "+strings.TrimSpace(out))
		return
	}

	auditFromRequest(r, "Group modified", name, "user")
	respondJSON(w, true, "Group '"+name+"' updated")
}

// setUserPassword sets a user's password using chpasswd
func setUserPassword(r *http.Request, username, password string) error {
	// Strip newlines to prevent chpasswd injection (e.g. "pass\nroot:evil")
	password = strings.ReplaceAll(password, "\n", "")
	password = strings.ReplaceAll(password, "\r", "")
	input := username + ":" + password
	if isSudoEnabled(r) {
		_, sudoPass, ok := getSessionPassword(r)
		if !ok {
			return fmt.Errorf("session expired")
		}
		out, err := sudoExecCommandStdin(sudoPass, input+"\n", "chpasswd")
		if err != nil {
			return fmt.Errorf("%s", strings.TrimSpace(out))
		}
		return nil
	}
	out, err := execCommandStdin(input+"\n", "chpasswd")
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return nil
}

// listUsers parses /etc/passwd and enriches with group and lock info
func listUsers(r *http.Request) []User {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil
	}
	return parseUsers(string(data), r)
}

func parseUsers(passwdContent string, r *http.Request) []User {
	var users []User
	for _, line := range strings.Split(passwdContent, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u := parsePasswdLine(line)
		if u == nil {
			continue
		}
		// Skip system users with UID < 1000 (except root)
		if u.UID < 1000 && u.UID != 0 {
			continue
		}
		// Get supplementary groups
		if groupOut, err := execCommand("id", "-nG", u.Username); err == nil {
			u.Groups = strings.TrimSpace(groupOut)
		}
		// Check if locked
		u.Locked = isUserLocked(r, u.Username)
		users = append(users, *u)
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].UID < users[j].UID
	})
	return users
}

func parsePasswdLine(line string) *User {
	fields := strings.Split(line, ":")
	if len(fields) < 7 {
		return nil
	}
	uid, _ := strconv.Atoi(fields[2])
	gid, _ := strconv.Atoi(fields[3])
	return &User{
		Username: fields[0],
		UID:      uid,
		GID:      gid,
		Comment:  fields[4],
		Home:     fields[5],
		Shell:    fields[6],
	}
}

func isUserLocked(r *http.Request, username string) bool {
	var out string
	if isSudoEnabled(r) {
		_, sudoPass, ok := getSessionPassword(r)
		if !ok {
			return false
		}
		out, _ = sudoExecCommand(sudoPass, "passwd", "-S", username)
	} else {
		out, _ = execCommand("passwd", "-S", username)
	}
	fields := strings.Fields(out)
	if len(fields) >= 2 {
		return fields[1] == "L"
	}
	return false
}

// listGroups parses /etc/group
func listGroups(r *http.Request) []Group {
	data, err := os.ReadFile("/etc/group")
	if err != nil {
		return nil
	}
	return parseGroups(string(data))
}

func parseGroups(groupContent string) []Group {
	var groups []Group
	for _, line := range strings.Split(groupContent, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		g := parseGroupLine(line)
		if g == nil {
			continue
		}
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].GID < groups[j].GID
	})
	return groups
}

func parseGroupLine(line string) *Group {
	fields := strings.Split(line, ":")
	if len(fields) < 4 {
		return nil
	}
	gid, _ := strconv.Atoi(fields[2])
	var members []string
	if fields[3] != "" {
		members = strings.Split(fields[3], ",")
	}
	return &Group{
		Name:    fields[0],
		GID:     gid,
		Members: members,
	}
}

// listShells reads /etc/shells for available login shells
func listShells() []string {
	data, err := os.ReadFile("/etc/shells")
	if err != nil {
		return []string{"/bin/bash", "/bin/sh", "/usr/sbin/nologin"}
	}
	var shells []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		shells = append(shells, line)
	}
	if len(shells) == 0 {
		return []string{"/bin/bash", "/bin/sh", "/usr/sbin/nologin"}
	}
	return shells
}
