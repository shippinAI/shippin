package main

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
)

type FirewallStatus struct {
	Active    bool           `json:"active"`
	Backend   string         `json:"backend"` // "ufw" or "iptables"
	Available bool           `json:"available"`
	Rules     []FirewallRule `json:"rules"`
}

type FirewallRule struct {
	Number    string `json:"number"`
	To        string `json:"to"`
	Action    string `json:"action"`
	From      string `json:"from"`
	Comment   string `json:"comment"`
	Raw       string `json:"raw"`
	Direction string `json:"direction"` // "IN" or "OUT"
}

func handleFirewall(w http.ResponseWriter, r *http.Request) {
	status := getFirewallStatus(r)

	hd := buildHeaderData(r)
	hd.PageTitle = "Firewall"
	data := struct {
		HeaderData
		Firewall FirewallStatus
	}{
		HeaderData: hd,
		Firewall:   status,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "firewall.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIFirewall(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		status := getFirewallStatus(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"firewall": status,
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
	case "enable":
		handleFirewallEnable(w, r)
	case "disable":
		handleFirewallDisable(w, r)
	case "add_rule":
		handleFirewallAddRule(w, r)
	case "delete_rule":
		handleFirewallDeleteRule(w, r)
	case "reset":
		handleFirewallReset(w, r)
	case "reload":
		handleFirewallReload(w, r)
	default:
		respondJSON(w, false, "Invalid action")
	}
}

func handleFirewallEnable(w http.ResponseWriter, r *http.Request) {
	// ufw needs --force to skip interactive prompt
	var out string
	var err error
	if isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(pass, "ufw", "--force", "enable")
	} else {
		out, err = execCommand("ufw", "--force", "enable")
	}
	if err != nil {
		respondJSON(w, false, "Failed to enable firewall: "+strings.TrimSpace(out))
		return
	}
	auditFromRequest(r, "Firewall enabled", "UFW enabled", "firewall")
	respondJSON(w, true, "Firewall enabled")
}

func handleFirewallDisable(w http.ResponseWriter, r *http.Request) {
	var out string
	var err error
	if isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(pass, "ufw", "disable")
	} else {
		out, err = execCommand("ufw", "disable")
	}
	if err != nil {
		respondJSON(w, false, "Failed to disable firewall: "+strings.TrimSpace(out))
		return
	}
	auditFromRequest(r, "Firewall disabled", "UFW disabled", "firewall")
	respondJSON(w, true, "Firewall disabled")
}

func handleFirewallAddRule(w http.ResponseWriter, r *http.Request) {
	ruleAction := r.FormValue("rule_action") // allow, deny, reject, limit
	direction := r.FormValue("direction")     // in, out
	port := r.FormValue("port")
	proto := r.FormValue("proto") // tcp, udp, any
	from := r.FormValue("from")
	to := r.FormValue("to")
	comment := r.FormValue("comment")

	if port == "" && from == "" && to == "" {
		respondJSON(w, false, "Port or address is required")
		return
	}
	if ruleAction == "" {
		ruleAction = "allow"
	}

	// Validate inputs
	if !isValidUFWAction(ruleAction) {
		respondJSON(w, false, "Invalid action: must be allow, deny, reject, or limit")
		return
	}
	if port != "" && !isValidPort(port) {
		respondJSON(w, false, "Invalid port specification")
		return
	}
	if from != "" && !isValidAddress(from) {
		respondJSON(w, false, "Invalid source address")
		return
	}
	if to != "" && !isValidAddress(to) {
		respondJSON(w, false, "Invalid destination address")
		return
	}

	args := buildUFWRuleArgs(ruleAction, direction, port, proto, from, to, comment)

	var out string
	var err error
	if isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(pass, "ufw", args...)
	} else {
		out, err = execCommand("ufw", args...)
	}
	if err != nil {
		respondJSON(w, false, "Failed to add rule: "+strings.TrimSpace(out))
		return
	}
	auditFromRequest(r, "Firewall rule added", ruleAction+" port "+port, "firewall")
	respondJSON(w, true, "Rule added")
}

func handleFirewallDeleteRule(w http.ResponseWriter, r *http.Request) {
	ruleNum := r.FormValue("rule_number")

	if ruleNum == "" {
		respondJSON(w, false, "Rule number is required")
		return
	}
	if !isValidPort(ruleNum) { // reuse: just digits
		respondJSON(w, false, "Invalid rule number")
		return
	}

	var out string
	var err error
	if isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(pass, "ufw", "--force", "delete", ruleNum)
	} else {
		out, err = execCommand("ufw", "--force", "delete", ruleNum)
	}
	if err != nil {
		respondJSON(w, false, "Failed to delete rule: "+strings.TrimSpace(out))
		return
	}
	auditFromRequest(r, "Firewall rule deleted", "Rule #"+ruleNum, "firewall")
	respondJSON(w, true, "Rule deleted")
}

func handleFirewallReset(w http.ResponseWriter, r *http.Request) {
	var out string
	var err error
	if isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(pass, "ufw", "--force", "reset")
	} else {
		out, err = execCommand("ufw", "--force", "reset")
	}
	if err != nil {
		respondJSON(w, false, "Failed to reset firewall: "+strings.TrimSpace(out))
		return
	}
	auditFromRequest(r, "Firewall reset", "Reset to defaults", "firewall")
	respondJSON(w, true, "Firewall reset to defaults")
}

func handleFirewallReload(w http.ResponseWriter, r *http.Request) {
	var out string
	var err error
	if isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(pass, "ufw", "reload")
	} else {
		out, err = execCommand("ufw", "reload")
	}
	if err != nil {
		respondJSON(w, false, "Failed to reload firewall: "+strings.TrimSpace(out))
		return
	}
	auditFromRequest(r, "Firewall reloaded", "UFW reloaded", "firewall")
	respondJSON(w, true, "Firewall reloaded")
}

func getFirewallStatus(r *http.Request) FirewallStatus {
	status := FirewallStatus{Backend: "ufw"}

	// Check if ufw is available
	if _, err := execCommand("which", "ufw"); err != nil {
		status.Available = false
		return status
	}
	status.Available = true

	// Get status
	var out string
	if isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			return status
		}
		out, _ = sudoExecCommand(pass, "ufw", "status", "numbered")
	} else {
		out, _ = execCommand("ufw", "status", "numbered")
	}

	status.Active = strings.Contains(out, "Status: active")
	status.Rules = parseUFWRules(out)
	return status
}

// parseUFWRules parses output from `ufw status numbered`
// Example lines:
// [ 1] 22/tcp                     ALLOW IN    Anywhere
// [ 2] 80,443/tcp                 ALLOW IN    Anywhere                   # Web server
// [ 3] Anywhere                   DENY IN     192.0.2.0/24
func parseUFWRules(output string) []FirewallRule {
	var rules []FirewallRule
	// Match lines like: [ 1] 22/tcp  ALLOW IN  Anywhere  # comment
	ruleRegex := regexp.MustCompile(`\[\s*(\d+)\]\s+(.+?)\s+(ALLOW|DENY|REJECT|LIMIT)\s+(IN|OUT|FWD)?\s*(.*)`)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		matches := ruleRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		rule := FirewallRule{
			Number:    matches[1],
			To:        strings.TrimSpace(matches[2]),
			Action:    matches[3],
			Direction: strings.TrimSpace(matches[4]),
			Raw:       line,
		}

		// Parse the "from" part and optional comment
		remainder := strings.TrimSpace(matches[5])
		if idx := strings.Index(remainder, "#"); idx >= 0 {
			rule.From = strings.TrimSpace(remainder[:idx])
			rule.Comment = strings.TrimSpace(remainder[idx+1:])
		} else {
			rule.From = remainder
		}

		rules = append(rules, rule)
	}
	return rules
}

func buildUFWRuleArgs(action, direction, port, proto, from, to, comment string) []string {
	args := []string{}

	// Simple rule: ufw allow 22/tcp
	// Complex rule: ufw allow from 10.0.0.0/8 to any port 22 proto tcp

	if from != "" || to != "" {
		// Complex rule syntax
		args = append(args, action)
		if direction != "" && direction != "in" {
			args = append(args, direction)
		}
		if from != "" {
			args = append(args, "from", from)
		}
		if to != "" {
			args = append(args, "to", to)
		} else if port != "" {
			args = append(args, "to", "any")
		}
		if port != "" {
			args = append(args, "port", port)
		}
		if proto != "" && proto != "any" && port != "" {
			args = append(args, "proto", proto)
		}
	} else {
		// Simple rule: ufw allow [in|out] port[/proto]
		args = append(args, action)
		if direction != "" && direction != "in" {
			args = append(args, direction)
		}
		portSpec := port
		if proto != "" && proto != "any" {
			portSpec = port + "/" + proto
		}
		args = append(args, portSpec)
	}

	if comment != "" {
		args = append(args, "comment", comment)
	}

	return args
}

// Validation helpers
var validPortRegex = regexp.MustCompile(`^[0-9,:]+(/[a-z]+)?$`)
var validAddressRegex = regexp.MustCompile(`^[0-9a-fA-F.:\/]+$|^any$|^Anywhere$`)

func isValidUFWAction(action string) bool {
	switch action {
	case "allow", "deny", "reject", "limit":
		return true
	}
	return false
}

func isValidPort(port string) bool {
	return validPortRegex.MatchString(port)
}

func isValidAddress(addr string) bool {
	return validAddressRegex.MatchString(addr)
}
