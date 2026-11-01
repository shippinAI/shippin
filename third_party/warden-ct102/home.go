package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

type PanelCard struct {
	Name   string
	URL    string
	Icon   string
	Desc   string
	Badge  string
	Status string // "green", "orange", "blue", "red", ""
	Group  string
	Size   string
}

type PanelGroup struct {
	Name  string
	Desc  string
	Cards []PanelCard
}

type HomeData struct {
	HeaderData
	Groups []PanelGroup
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	cards := buildPanelCards(r)

	data := HomeData{
		HeaderData: buildHeaderData(r),
		Groups:     groupPanelCards(cards),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "home.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func buildPanelCards(r *http.Request) []PanelCard {
	cards := []PanelCard{
		{Name: "Dashboard", URL: "/dashboard", Icon: "\U0001F4CA", Desc: "Health, resource pressure, and estate posture", Group: "Overview", Size: "lg"},
		{Name: "Agent Workspaces", URL: "/agent-workspaces", Icon: "\U0001F5A5", Desc: "Launch Codex, Claude, and editor sessions", Group: "Overview", Size: "lg"},
		{Name: "Workspace Map", URL: "/workspaces", Icon: "\U0001F5FA", Desc: "WardenClyffe, Clyffy, projects, and explicit edges", Group: "Overview", Size: "lg"},
		{Name: "Catalog", URL: "/catalog", Icon: "\U0001F4DA", Desc: "Safe service templates ready to deploy", Group: "Deploy", Size: "lg"},
		{Name: "Deployments", URL: "/deployments", Icon: "\U0001F680", Desc: "Rollouts, recovery, and automation status", Group: "Deploy", Size: "lg"},
		{Name: "Customers", URL: "/customers", Icon: "\U0001F4BC", Desc: "Customer accounts, service auth, and drill-down posture", Group: "Deploy", Size: "md"},
		{Name: "Warden Internal", URL: "/internal", Icon: "\U0001F6E0\uFE0F", Desc: "Platform-owned control-plane and shared services", Group: "Deploy", Size: "md"},
		{Name: "Proxmox", URL: "/proxmox", Icon: "\U0001F5A5", Desc: "Host, VM, and container substrate", Group: "Infrastructure", Size: "lg"},
		{Name: "Runners", URL: "/runners", Icon: "\U0001F3C3", Desc: "CI fleet health, build streaks, and The Race", Group: "Infrastructure", Size: "md"},
		{Name: "Network", URL: "/network", Icon: "\U0001F5A7", Desc: "Interfaces, connections, and ports", Group: "Infrastructure", Size: "md"},
		{Name: "Disks", URL: "/disks", Icon: "\U0001F4BD", Desc: "Partitions, SMART health, and usage", Group: "Infrastructure", Size: "md"},
		{Name: "DNS", URL: "/dns", Icon: "\U0001F310", Desc: "Domain estate, zones, routes, and resolver config", Group: "Infrastructure", Size: "sm"},
		{Name: "Certificates", URL: "/certs", Icon: "\U0001F512", Desc: "SSL/TLS certificates and renewal posture", Group: "Infrastructure", Size: "sm"},
		{Name: "Users", URL: "/users", Icon: "\U0001F465", Desc: "System user accounts", Group: "Access", Size: "md"},
		{Name: "SSH Keys", URL: "/sshkeys", Icon: "\U0001F511", Desc: "Authorized keys and shell access", Group: "Access", Size: "md"},
		{Name: "Firewall", URL: "/firewall", Icon: "\U0001F6E1\uFE0F", Desc: "Rules, ports, and network security", Group: "Access", Size: "md"},
		{Name: "Services", URL: "/services", Icon: "\u2699\uFE0F", Desc: "Start, stop, and inspect system services", Group: "Operations", Size: "lg"},
		{Name: "Packages", URL: "/packages", Icon: "\U0001F4E6", Desc: "System packages and updates", Group: "Operations", Size: "md"},
		{Name: "Docker", URL: "/docker", Icon: "\U0001F433", Desc: "Containers, images, volumes, and networks", Group: "Operations", Size: "lg"},
		{Name: "Files", URL: "/files", Icon: "\U0001F4C1", Desc: "Browse and manage files", Group: "Operations", Size: "md"},
		{Name: "Cron", URL: "/cron", Icon: "\u23F0", Desc: "Scheduled jobs and recurring maintenance", Group: "Operations", Size: "sm"},
		{Name: "Backups", URL: "/backups", Icon: "\U0001F4BE", Desc: "Full backup to shippinS3 (SystemFreaks) with live progress", Group: "Operations", Size: "md"},
		{Name: "Logs", URL: "/logs", Icon: "\U0001F4DC", Desc: "Service logs and failure detail", Group: "Observability", Size: "md"},
		{Name: "Endpoints", URL: "/endpoints", Icon: "\U0001F4E1", Desc: "Availability checks and uptime history", Group: "Observability", Size: "md"},
		{Name: "Notifications", URL: "/notifications", Icon: "\U0001F514", Desc: "Webhook and email alert rules", Group: "Observability", Size: "sm"},
		{Name: "Audit Log", URL: "/audit", Icon: "\U0001F4CB", Desc: "Who changed what and when", Group: "Observability", Size: "md"},
		{Name: "System Info", URL: "/sysinfo", Icon: "\U0001F5A5", Desc: "Hostname, OS, CPU, RAM, and specs", Group: "Settings", Size: "sm"},
		{Name: "Config", URL: "/config", Icon: "\U0001F4E5", Desc: "Export and import Warden settings", Group: "Settings", Size: "md"},
		{Name: "System", URL: "/system", Icon: "\u26A1", Desc: "Reboot, shutdown, and power actions", Group: "Settings", Size: "sm"},
		{Name: "Terminal", URL: "/terminal", Icon: "\U0001F4BB", Desc: "Expert shell access", Group: "Settings", Size: "sm"},
		{Name: "Help", URL: "/help", Icon: "\u2753", Desc: "Documentation and about", Group: "Settings", Size: "sm"},
	}

	// Helper to set badge by card name
	setBadge := func(name, badge, status string) {
		for i := range cards {
			if cards[i].Name == name {
				cards[i].Badge = badge
				cards[i].Status = status
				return
			}
		}
	}

	// Catalog (template count)
	if catInv := getCatalog(); catInv.Available {
		if catInv.OkCount > 0 {
			setBadge("Catalog", fmt.Sprintf("%d ready", catInv.OkCount), "green")
		}
		if catInv.ErrCount > 0 {
			setBadge("Catalog", fmt.Sprintf("%d ready, %d error", catInv.OkCount, catInv.ErrCount), "orange")
		}
	}

	// Deployments
	if deps := listDeployments(); len(deps) > 0 {
		running := 0
		customerOwned := 0
		for _, d := range deps {
			if d.Status == DeploymentRunning {
				running++
			}
			if strings.TrimSpace(d.CustomerSlug) != "" {
				customerOwned++
			}
		}
		if running > 0 {
			setBadge("Deployments", fmt.Sprintf("%d total, %d running", len(deps), running), "blue")
		} else {
			setBadge("Deployments", fmt.Sprintf("%d tracked", len(deps)), "green")
		}
		setBadge("Customers", fmt.Sprintf("%d tenant-owned", customerOwned), "blue")
		setBadge("Warden Internal", fmt.Sprintf("%d platform-owned", len(deps)-customerOwned), "green")
	}

	// Services
	svcOut, _ := execCommand("systemctl", "list-units", "--type=service", "--state=running", "--no-legend", "--no-pager")
	if svcOut = strings.TrimSpace(svcOut); svcOut != "" {
		setBadge("Services", formatCount(len(strings.Split(svcOut, "\n")), "running"), "green")
	}

	// Docker containers
	dkOut, err := execCommand("docker", "ps", "--format", "{{.ID}}")
	if err == nil && strings.TrimSpace(dkOut) != "" {
		setBadge("Docker", formatCount(len(strings.Split(strings.TrimSpace(dkOut), "\n")), "running"), "blue")
	}

	// Cron jobs
	cronOut, _ := execCommand("bash", "-c", "crontab -l 2>/dev/null | grep -v '^#' | grep -v '^$' | wc -l")
	if cronCount := strings.TrimSpace(cronOut); cronCount != "" && cronCount != "0" {
		setBadge("Cron", cronCount+" jobs", "blue")
	}

	backupMu.RLock()
	estateRunning := runningJobs[estateJobIDFull]
	lastEstate := ""
	for _, run := range backupStore.History {
		if run.Kind == "estate" && run.Status == "success" {
			lastEstate = run.EndTime
			break
		}
	}
	backupMu.RUnlock()
	if estateRunning {
		setBadge("Backups", "running", "blue")
	} else if lastEstate != "" {
		setBadge("Backups", "last "+lastEstate, "green")
	} else {
		setBadge("Backups", "not backed up", "orange")
	}

	// Firewall
	fwStatus := getFirewallStatus(r)
	if fwStatus.Available {
		if fwStatus.Active {
			setBadge("Firewall", "Active", "green")
		} else {
			setBadge("Firewall", "Inactive", "red")
		}
	}

	// Domain/DNS registry
	if reg := getDomainRegistry(); reg.Available {
		status := "green"
		if reg.Summary.PendingCount > 0 {
			status = "orange"
		}
		setBadge("DNS", fmt.Sprintf("%d domains, %d routes", reg.Summary.DomainCount, reg.Summary.RouteCount), status)
	}

	// Managed workspace registry
	if workspaceMap := getManagedWorkspaceMap(); workspaceMap.Available {
		setBadge("Workspace Map", fmt.Sprintf("%d workspaces, %d edges",
			workspaceMap.Summary.WorkspaceCount, workspaceMap.Summary.ConnectionCount), "blue")
	}

	// Endpoints
	endpointsMu.RLock()
	epCount := len(endpoints)
	upCount := 0
	downCount := 0
	for _, ep := range endpoints {
		if s, ok := endpointStatuses[ep.ID]; ok && s.TotalChecks > 0 {
			if s.Up {
				upCount++
			} else {
				downCount++
			}
		}
	}
	endpointsMu.RUnlock()
	if epCount > 0 {
		status := "green"
		badge := fmt.Sprintf("%d up", upCount)
		if downCount > 0 {
			status = "red"
			badge = fmt.Sprintf("%d up, %d down", upCount, downCount)
		}
		setBadge("Endpoints", badge, status)
	}

	return cards
}

func formatCount(n int, label string) string {
	return fmt.Sprintf("%d %s", n, label)
}

func groupPanelCards(cards []PanelCard) []PanelGroup {
	groups := []PanelGroup{
		{Name: "Overview", Desc: "What needs attention now"},
		{Name: "Deploy", Desc: "What can be installed and what is rolling out"},
		{Name: "Infrastructure", Desc: "Hosts, routes, storage, and network posture"},
		{Name: "Access", Desc: "Who can enter and what is exposed"},
		{Name: "Operations", Desc: "Daily maintenance and host actions"},
		{Name: "Observability", Desc: "Failures, changes, and notifications"},
		{Name: "Settings", Desc: "Configuration and expert tools"},
	}
	byName := map[string]int{}
	for i := range groups {
		byName[groups[i].Name] = i
	}
	for _, c := range cards {
		idx, ok := byName[c.Group]
		if !ok {
			idx = byName["Settings"]
		}
		groups[idx].Cards = append(groups[idx].Cards, c)
	}
	out := groups[:0]
	for _, g := range groups {
		if len(g.Cards) > 0 {
			out = append(out, g)
		}
	}
	return out
}
