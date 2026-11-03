package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type DNSInfo struct {
	Hosts          []HostEntry        `json:"hosts"`
	Nameservers    []string           `json:"nameservers"`
	SearchDomains  []string           `json:"search_domains"`
	ResolvConf     string             `json:"resolv_conf"`
	ServiceName    string             `json:"service_name"`
	Running        bool               `json:"running"`
	Available      bool               `json:"available"`
	DomainRegistry DomainRegistryView `json:"domain_registry"`
}

type HostEntry struct {
	Line      int    `json:"line"`
	IP        string `json:"ip"`
	Hosts     string `json:"hosts"`
	Comment   string `json:"comment"`
	Raw       string `json:"raw"`
	IsComment bool   `json:"is_comment"`
}

func handleDNS(w http.ResponseWriter, r *http.Request) {
	dns := getDNSInfo(r)

	hd := buildHeaderData(r)
	hd.PageTitle = "DNS"
	data := struct {
		HeaderData
		DNS DNSInfo
	}{
		HeaderData: hd,
		DNS:        dns,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "dns.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIDNS(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		dns := getDNSInfo(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"dns":     dns,
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
	case "add_host":
		handleDNSAddHost(w, r)
	case "delete_host":
		handleDNSDeleteHost(w, r)
	case "edit_host":
		handleDNSEditHost(w, r)
	case "set_nameservers":
		handleDNSSetNameservers(w, r)
	case "restart":
		handleDNSServiceRestart(w, r)
	default:
		respondJSON(w, false, "Invalid action")
	}
}

// Validation
var hostIPRegex = regexp.MustCompile(`^[0-9a-fA-F.:]+$`)
var hostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func handleDNSAddHost(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimSpace(r.FormValue("ip"))
	hosts := strings.TrimSpace(r.FormValue("hosts"))
	comment := strings.TrimSpace(r.FormValue("comment"))

	if ip == "" || hosts == "" {
		respondJSON(w, false, "IP address and hostname are required")
		return
	}
	if !hostIPRegex.MatchString(ip) {
		respondJSON(w, false, "Invalid IP address format")
		return
	}
	for _, h := range strings.Fields(hosts) {
		if !hostnameRegex.MatchString(h) {
			respondJSON(w, false, "Invalid hostname: "+h)
			return
		}
	}

	line := ip + "\t" + hosts
	if comment != "" {
		line += " # " + comment
	}

	// Append to /etc/hosts
	var out string
	var err error
	if isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, err = sudoExecCommand(pass, "bash", "-c", "echo "+shellQuote(line)+" >> /etc/hosts")
	} else {
		out, err = execCommand("bash", "-c", "echo "+shellQuote(line)+" >> /etc/hosts")
	}
	if err != nil {
		respondJSON(w, false, "Failed to add host entry: "+strings.TrimSpace(out))
		return
	}
	auditFromRequest(r, "DNS host added", ip+" "+hosts, "dns")
	respondJSON(w, true, "Host entry added")
}

func handleDNSDeleteHost(w http.ResponseWriter, r *http.Request) {
	lineStr := r.FormValue("line")
	lineNum := 0
	fmt.Sscanf(lineStr, "%d", &lineNum)
	if lineNum < 1 {
		respondJSON(w, false, "Invalid line number")
		return
	}

	content, err := readHostsFile(r)
	if err != nil {
		respondJSON(w, false, "Failed to read /etc/hosts: "+err.Error())
		return
	}

	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if lineNum > len(lines) {
		respondJSON(w, false, "Line number out of range")
		return
	}

	newLines := append(lines[:lineNum-1], lines[lineNum:]...)
	newContent := strings.Join(newLines, "\n") + "\n"

	if err := writeHostsFile(r, newContent); err != nil {
		respondJSON(w, false, "Failed to update /etc/hosts: "+err.Error())
		return
	}
	auditFromRequest(r, "DNS host deleted", "Line "+lineStr, "dns")
	respondJSON(w, true, "Host entry deleted")
}

func handleDNSEditHost(w http.ResponseWriter, r *http.Request) {
	lineStr := r.FormValue("line")
	ip := strings.TrimSpace(r.FormValue("ip"))
	hosts := strings.TrimSpace(r.FormValue("hosts"))
	comment := strings.TrimSpace(r.FormValue("comment"))

	lineNum := 0
	fmt.Sscanf(lineStr, "%d", &lineNum)
	if lineNum < 1 {
		respondJSON(w, false, "Invalid line number")
		return
	}
	if ip == "" || hosts == "" {
		respondJSON(w, false, "IP address and hostname are required")
		return
	}
	if !hostIPRegex.MatchString(ip) {
		respondJSON(w, false, "Invalid IP address format")
		return
	}
	for _, h := range strings.Fields(hosts) {
		if !hostnameRegex.MatchString(h) {
			respondJSON(w, false, "Invalid hostname: "+h)
			return
		}
	}

	content, err := readHostsFile(r)
	if err != nil {
		respondJSON(w, false, "Failed to read /etc/hosts: "+err.Error())
		return
	}

	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if lineNum > len(lines) {
		respondJSON(w, false, "Line number out of range")
		return
	}

	newLine := ip + "\t" + hosts
	if comment != "" {
		newLine += " # " + comment
	}
	lines[lineNum-1] = newLine
	newContent := strings.Join(lines, "\n") + "\n"

	if err := writeHostsFile(r, newContent); err != nil {
		respondJSON(w, false, "Failed to update /etc/hosts: "+err.Error())
		return
	}
	auditFromRequest(r, "DNS host edited", ip+" "+hosts, "dns")
	respondJSON(w, true, "Host entry updated")
}

func handleDNSSetNameservers(w http.ResponseWriter, r *http.Request) {
	nameservers := strings.TrimSpace(r.FormValue("nameservers"))
	searchDomains := strings.TrimSpace(r.FormValue("search"))

	if nameservers == "" {
		respondJSON(w, false, "At least one nameserver is required")
		return
	}

	// Validate nameserver IPs
	for _, ns := range strings.Fields(nameservers) {
		if !hostIPRegex.MatchString(ns) {
			respondJSON(w, false, "Invalid nameserver IP: "+ns)
			return
		}
	}

	// Validate search domains (letters, digits, dots, hyphens only — no newlines or shell chars)
	if searchDomains != "" {
		for _, sd := range strings.Fields(searchDomains) {
			if !hostnameRegex.MatchString(sd) {
				respondJSON(w, false, "Invalid search domain: "+sd)
				return
			}
		}
	}

	// Build resolv.conf content
	var lines []string
	lines = append(lines, "# Generated by Warden")
	if searchDomains != "" {
		lines = append(lines, "search "+searchDomains)
	}
	for _, ns := range strings.Fields(nameservers) {
		lines = append(lines, "nameserver "+ns)
	}
	newContent := strings.Join(lines, "\n") + "\n"

	var out string
	var writeErr error
	if isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			respondJSON(w, false, "Session expired")
			return
		}
		out, writeErr = sudoExecCommand(pass, "bash", "-c", "echo "+shellQuote(strings.TrimRight(newContent, "\n"))+" > /etc/resolv.conf")
	} else {
		out, writeErr = execCommand("bash", "-c", "echo "+shellQuote(strings.TrimRight(newContent, "\n"))+" > /etc/resolv.conf")
	}
	if writeErr != nil {
		respondJSON(w, false, "Failed to update resolv.conf: "+strings.TrimSpace(out))
		return
	}
	auditFromRequest(r, "DNS nameservers updated", nameservers, "dns")
	respondJSON(w, true, "DNS nameservers updated")
}

func handleDNSServiceRestart(w http.ResponseWriter, r *http.Request) {
	svcName := detectDNSService()
	if svcName == "" {
		respondJSON(w, false, "No DNS service detected")
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
		out, err = sudoExecCommand(pass, "systemctl", "restart", svcName)
	} else {
		out, err = execCommand("systemctl", "restart", svcName)
	}
	if err != nil {
		respondJSON(w, false, "Failed to restart "+svcName+": "+strings.TrimSpace(out))
		return
	}
	auditFromRequest(r, "DNS service restarted", svcName, "dns")
	respondJSON(w, true, svcName+" restarted")
}

func getDNSInfo(r *http.Request) DNSInfo {
	info := DNSInfo{
		DomainRegistry: getDomainRegistry(),
	}

	// Parse /etc/hosts
	content, err := readHostsFile(r)
	if err == nil {
		info.Hosts = parseHostsFile(content)
	}

	// Parse /etc/resolv.conf
	resolvData, err := os.ReadFile("/etc/resolv.conf")
	if err == nil {
		info.ResolvConf = string(resolvData)
		info.Nameservers, info.SearchDomains = parseResolvConf(string(resolvData))
	}

	// Check for DNS service
	info.ServiceName = detectDNSService()
	if info.ServiceName != "" {
		info.Available = true
		out, _ := execCommand("systemctl", "is-active", info.ServiceName)
		info.Running = strings.TrimSpace(out) == "active"
	}

	return info
}

func parseHostsFile(content string) []HostEntry {
	var entries []HostEntry
	for i, line := range strings.Split(content, "\n") {
		raw := line
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		entry := HostEntry{
			Line: i + 1,
			Raw:  raw,
		}

		if strings.HasPrefix(line, "#") {
			entry.IsComment = true
			entry.Comment = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			entries = append(entries, entry)
			continue
		}

		// Split comment from line
		commentIdx := strings.Index(line, "#")
		datapart := line
		if commentIdx >= 0 {
			entry.Comment = strings.TrimSpace(line[commentIdx+1:])
			datapart = strings.TrimSpace(line[:commentIdx])
		}

		fields := strings.Fields(datapart)
		if len(fields) < 2 {
			entry.IsComment = true
			entry.Comment = line
			entries = append(entries, entry)
			continue
		}

		entry.IP = fields[0]
		entry.Hosts = strings.Join(fields[1:], " ")
		entries = append(entries, entry)
	}
	return entries
}

func parseResolvConf(content string) (nameservers []string, searchDomains []string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if strings.HasPrefix(line, "nameserver ") {
			ns := strings.TrimSpace(strings.TrimPrefix(line, "nameserver "))
			if ns != "" {
				nameservers = append(nameservers, ns)
			}
		}
		if strings.HasPrefix(line, "search ") {
			parts := strings.Fields(line)
			if len(parts) > 1 {
				searchDomains = append(searchDomains, parts[1:]...)
			}
		}
	}
	return
}

func detectDNSService() string {
	// Check common DNS services
	for _, svc := range []string{"systemd-resolved", "bind9", "named", "dnsmasq", "unbound"} {
		out, _ := execCommand("systemctl", "list-unit-files", svc+".service")
		if strings.Contains(out, svc+".service") {
			return svc
		}
	}
	return ""
}

func readHostsFile(r *http.Request) (string, error) {
	if isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			return "", fmt.Errorf("session expired")
		}
		out, err := sudoExecCommand(pass, "cat", "/etc/hosts")
		if err != nil {
			return "", fmt.Errorf("%s", strings.TrimSpace(out))
		}
		return out, nil
	}
	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func writeHostsFile(r *http.Request, content string) error {
	if isSudoEnabled(r) {
		_, pass, ok := getSessionPassword(r)
		if !ok {
			return fmt.Errorf("session expired")
		}
		out, err := sudoExecCommand(pass, "bash", "-c", "echo "+shellQuote(strings.TrimRight(content, "\n"))+" > /etc/hosts")
		if err != nil {
			return fmt.Errorf("%s", strings.TrimSpace(out))
		}
		return nil
	}
	out, err := execCommand("bash", "-c", "echo "+shellQuote(strings.TrimRight(content, "\n"))+" > /etc/hosts")
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return nil
}
