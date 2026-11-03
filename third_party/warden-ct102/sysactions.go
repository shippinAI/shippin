package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

func handleSystemActions(w http.ResponseWriter, r *http.Request) {
	hd := buildHeaderData(r)
	hd.PageTitle = "System"
	data := struct {
		HeaderData
		Message string
	}{
		HeaderData: hd,
		Message:    r.URL.Query().Get("msg"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "sysactions.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleSystemActionAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.ParseForm()
	action := r.FormValue("action")

	validActions := map[string]bool{
		"reboot": true, "shutdown": true, "poweroff": true,
	}
	if !validActions[action] {
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	if !isSudoEnabled(r) {
		respond(w, r, false, "Sudo must be enabled for system actions", "system")
		return
	}

	_, password, ok := getSessionPassword(r)
	if !ok {
		respond(w, r, false, "Session expired", "system")
		return
	}

	var args []string
	switch action {
	case "reboot":
		args = []string{"systemctl", "reboot"}
	case "shutdown":
		args = []string{"systemctl", "poweroff", "--when=+1"}
	case "poweroff":
		args = []string{"systemctl", "poweroff"}
	}

	username, _ := getSession(r)
	log.Printf("system action %q requested by user %q", action, username)

	out, err := sudoExecCommand(password, args[0], args[1:]...)
	msg := action + ": initiated"
	success := true
	if err != nil {
		msg = action + ": " + strings.TrimSpace(out)
		if msg == action+": " {
			msg = action + ": failed"
		}
		success = false
	}

	if success {
		auditFromRequest(r, "System "+action, action, "system")
	}

	respond(w, r, success, msg, "system")
}

func respond(w http.ResponseWriter, r *http.Request, success bool, msg, tab string) {
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": success,
			"message": msg,
		})
		return
	}
	http.Redirect(w, r, "/system?msg="+msg, http.StatusSeeOther)
}
