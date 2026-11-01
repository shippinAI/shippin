package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func handleSudoToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	enabled := toggleSudo(r)
	user, _ := getSession(r)
	log.Printf("user %q toggled sudo: %v", user, enabled)

	sudoMsg := "Sudo disabled"
	if enabled {
		sudoMsg = "Sudo enabled"
	}
	auditFromRequest(r, "Sudo toggled", sudoMsg, "auth")

	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": sudoMsg,
			"enabled": enabled,
		})
		return
	}

	ref := r.Header.Get("Referer")
	if ref == "" {
		ref = "/"
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if _, ok := getSession(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	type loginData struct {
		Error   string
		Version string
		OIDC    OIDCConfig
	}

	if r.Method == http.MethodGet {
		data := loginData{Version: Version, OIDC: getOIDCConfig(r)}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.ExecuteTemplate(w, "login.html", data); err != nil {
			log.Printf("template error: %v", err)
		}
		return
	}

	r.ParseForm()
	username := r.FormValue("username")
	password := r.FormValue("password")
	clientIP := getClientIP(r)

	if username == "" || password == "" {
		renderLoginError(w, r, "Username and password are required", http.StatusOK)
		return
	}

	if isRateLimited(clientIP) {
		log.Printf("login rate limited for IP %s (user %q)", clientIP, username)
		addAuditEntry(username, "Login blocked", "Rate limited", clientIP, "auth")
		renderLoginError(w, r, "Too many failed attempts. Please try again in a few minutes.", http.StatusTooManyRequests)
		return
	}

	if err := pamAuthenticate(username, password); err != nil {
		recordFailedLogin(clientIP)
		log.Printf("auth failure for user %q from %s: %v", username, clientIP, err)
		addAuditEntry(username, "Login failed", "Invalid credentials", clientIP, "auth")
		renderLoginError(w, r, "Invalid username or password", http.StatusOK)
		return
	}

	clearFailedLogins(clientIP)
	token := createSession(username, password)
	setSessionCookie(w, token)

	log.Printf("user %q logged in from %s", username, clientIP)
	addAuditEntry(username, "Login", "Logged in", clientIP, "auth")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func renderLoginError(w http.ResponseWriter, r *http.Request, msg string, status int) {
	data := struct {
		Error   string
		Version string
		OIDC    OIDCConfig
	}{Error: msg, Version: Version, OIDC: getOIDCConfig(r)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	if err := templates.ExecuteTemplate(w, "login.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !checkCSRFOrigin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if user, ok := getSession(r); ok {
		log.Printf("user %q logged out", user)
	}
	auditFromRequest(r, "Logout", "Logged out", "auth")
	destroySession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
