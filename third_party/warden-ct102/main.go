package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const Version = "0.32-beta"

//go:embed templates/*.html components/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

var templates *template.Template

func init() {
	funcMap := template.FuncMap{
		"formatBytes":    formatBytes,
		"formatUptime":   formatUptime,
		"printf":         fmt.Sprintf,
		"cpuClass":       levelClass,
		"memClass":       levelClass,
		"diskClass":      levelClass,
		"levelTextClass": func(pct float64) string { return levelClass(pct) + "-text" },
		"divFloat": func(a, b int64) float64 {
			if b == 0 {
				return 0
			}
			return float64(a) / float64(b)
		},
		"mulInt":   func(a int64, b int) int64 { return a * int64(b) },
		"mulFloat": func(a, b float64) float64 { return a * b },
		"joinTags": joinTags,
		"tempClass": func(temp float64) string {
			switch {
			case temp >= 90:
				return "temp-crit"
			case temp >= 75:
				return "temp-high"
			case temp >= 60:
				return "temp-warn"
			default:
				return "temp-ok"
			}
		},
		"totalDiskPerc": func(disks []DiskInfo) float64 {
			var totalSize, totalUsed uint64
			for _, d := range disks {
				totalSize += d.Total
				totalUsed += d.Used
			}
			if totalSize == 0 {
				return 0
			}
			return float64(totalUsed) / float64(totalSize) * 100
		},
		"totalDiskUsed": func(disks []DiskInfo) uint64 {
			var total uint64
			for _, d := range disks {
				total += d.Used
			}
			return total
		},
		"totalDiskSize": func(disks []DiskInfo) uint64 {
			var total uint64
			for _, d := range disks {
				total += d.Total
			}
			return total
		},
		"formatBytesT":   formatBytes,
		"contains":       strings.Contains,
		"formatFileSize": formatFileSize,
		"isTextFile":     isTextFile,
		"breadcrumbs": func(dir string) []struct{ Name, Path string } {
			parts := strings.Split(filepath.Clean(dir), "/")
			var crumbs []struct{ Name, Path string }
			crumbs = append(crumbs, struct{ Name, Path string }{"/", "/"})
			path := ""
			for _, p := range parts {
				if p == "" {
					continue
				}
				path += "/" + p
				crumbs = append(crumbs, struct{ Name, Path string }{p, path})
			}
			return crumbs
		},
		"stateClass": func(state string) string {
			switch strings.ToLower(state) {
			case "up":
				return "state-up"
			case "down":
				return "state-down"
			default:
				return "state-unknown"
			}
		},
	}
	for name, fn := range componentFuncMap {
		funcMap[name] = fn
	}
	templates = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html", "components/*.html"))
}

// VersionInfo holds current and latest version details.
type VersionInfo struct {
	Current     string    `json:"current"`
	Latest      string    `json:"latest"`
	UpdateAvail bool      `json:"update_available"`
	ReleaseURL  string    `json:"release_url"`
	CheckedAt   time.Time `json:"checked_at"`
}

var (
	cachedVersion        VersionInfo
	versionMu            sync.RWMutex
	versionCheckInterval = 1 * time.Hour
)

func init() {
	go versionCheckLoop()
}

func versionCheckLoop() {
	checkLatestVersion()
	for range time.Tick(versionCheckInterval) {
		checkLatestVersion()
	}
}

func checkLatestVersion() {
	client := &http.Client{Timeout: 10 * time.Second}
	// Wardenclyffe is a hard fork. We track our own GitHub releases, not upstream Anvil.
	resp, err := client.Get("https://api.github.com/repos/example-org/wardenclyffe/releases/latest")
	if err != nil {
		log.Printf("version check failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// No releases published yet on the repo. Cache current version with no update.
		versionMu.Lock()
		cachedVersion = VersionInfo{
			Current:   Version,
			Latest:    Version,
			CheckedAt: time.Now(),
		}
		versionMu.Unlock()
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &release); err != nil || release.TagName == "" {
		return
	}

	latestTag := strings.TrimPrefix(release.TagName, "v")

	versionMu.Lock()
	cachedVersion = VersionInfo{
		Current:     Version,
		Latest:      latestTag,
		UpdateAvail: isNewer(latestTag, Version),
		ReleaseURL:  release.HTMLURL,
		CheckedAt:   time.Now(),
	}
	versionMu.Unlock()
	log.Printf("version check: current=%s latest=%s update=%v", Version, latestTag, isNewer(latestTag, Version))
}

// isNewer returns true if remote is a higher version than local.
// Compares numeric segments (e.g. "0.12-beta" vs "0.11-beta").
func isNewer(remote, local string) bool {
	parse := func(v string) []int {
		v = strings.TrimPrefix(v, "v")
		// Strip suffix like "-beta", "-rc1"
		if idx := strings.IndexAny(v, "-+"); idx != -1 {
			v = v[:idx]
		}
		var parts []int
		for _, s := range strings.Split(v, ".") {
			n := 0
			for _, c := range s {
				if c >= '0' && c <= '9' {
					n = n*10 + int(c-'0')
				}
			}
			parts = append(parts, n)
		}
		return parts
	}
	r, l := parse(remote), parse(local)
	for i := 0; i < len(r) || i < len(l); i++ {
		rv, lv := 0, 0
		if i < len(r) {
			rv = r[i]
		}
		if i < len(l) {
			lv = l[i]
		}
		if rv > lv {
			return true
		}
		if rv < lv {
			return false
		}
	}
	return false
}

func getVersionInfo() VersionInfo {
	versionMu.RLock()
	defer versionMu.RUnlock()
	if cachedVersion.Current == "" {
		return VersionInfo{Current: Version}
	}
	return cachedVersion
}

// HeaderData holds the common fields shared across all page templates.
type HeaderData struct {
	Username       string
	SudoEnabled    bool
	Version        VersionInfo
	Hostname       string
	CPU            CPUInfo
	Memory         MemInfo
	Disks          []DiskInfo
	UptimeFmt      string
	PageTitle      string
	InfraProviders int
	InfraCustomers int
	InfraResources int
}

func buildHeaderData(r *http.Request) HeaderData {
	username, _ := getSession(r)
	hostname, _ := os.Hostname()
	stats := collectStats()
	graph := getInfrastructureGraph(stats)
	return HeaderData{
		Username:       username,
		SudoEnabled:    isSudoEnabled(r),
		Version:        getVersionInfo(),
		Hostname:       hostname,
		CPU:            stats.CPU,
		Memory:         stats.Memory,
		Disks:          stats.Disks,
		UptimeFmt:      formatUptime(stats.Uptime),
		InfraProviders: graph.Summary.Providers,
		InfraCustomers: graph.Summary.Customers,
		InfraResources: graph.Summary.Resources,
	}
}

// SystemStats holds all collected system statistics.
type SystemStats struct {
	Username       string
	SudoEnabled    bool
	Version        VersionInfo
	Hostname       string
	UptimeFmt      string
	PageTitle      string
	OS             string
	Kernel         string
	Arch           string
	GoVersion      string
	Uptime         time.Duration
	LoadAvg        LoadAverage
	CPU            CPUInfo
	Memory         MemInfo
	Swap           MemInfo
	Disks          []DiskInfo
	Network        []NetInfo
	TopProcs       []ProcInfo
	LoggedIn       []LoggedInUser
	Temps          []TempSensor
	Listeners      []ListenPort
	DiskIO         []DiskIOInfo
	RecentLogins   []RecentLogin
	ProcessCount   int
	PackageUpdates PackageInfo
	CollectedAt    time.Time
	DashboardTabs  []DashboardTab
	MainCharts     []ChartPerspective
	HealthWidgets  []HealthWidget
	HostNodes      []HostNode
}

type LoadAverage struct {
	Load1, Load5, Load15 string
}

type CPUInfo struct {
	ModelName  string
	Cores      int
	UsagePerc  float64
	UserPerc   float64
	SystemPerc float64
	IdlePerc   float64
}

type MemInfo struct {
	Total     uint64
	Used      uint64
	Free      uint64
	Available uint64
	UsedPerc  float64
}

type DiskInfo struct {
	Device     string
	MountPoint string
	FSType     string
	Total      uint64
	Used       uint64
	Free       uint64
	UsedPerc   float64
}

type NetInfo struct {
	Name    string
	RxBytes uint64
	TxBytes uint64
	State   string
	IPv4    string
}

type ProcInfo struct {
	PID     int
	User    string
	CPU     float64
	Mem     float64
	Command string
}

type LoggedInUser struct {
	User     string
	Terminal string
	From     string
	LoginAt  string
}

type TempSensor struct {
	Label string
	Temp  float64
}

type ListenPort struct {
	Proto   string
	Address string
	Port    int
	PID     int
	Process string
}

type DiskIOInfo struct {
	Device          string
	ReadsCompleted  uint64
	WritesCompleted uint64
	ReadBytes       uint64
	WriteBytes      uint64
}

type RecentLogin struct {
	User    string
	From    string
	Time    string
	Success bool
}

type PackageInfo struct {
	Total    int
	Security int
}

type SystemdUnit struct {
	Name        string
	Description string
	UnitType    string
	ActiveState string
	SubState    string
	Enabled     string
	Running     bool
}

func main() {
	port := "9006"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	tlsCert := os.Getenv("TLS_CERT")
	tlsKey := os.Getenv("TLS_KEY")
	tlsEnabled := os.Getenv("TLS") == "true" || tlsCert != ""
	secureCookies = tlsEnabled

	initAudit()
	startSessionCleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := initSupabase(ctx); err != nil {
		if supabaseRequired() {
			log.Fatalf("Supabase init failed: %v", err)
		}
		log.Printf("Supabase init failed; continuing in degraded mode: %v", err)
	} else {
		defer closeSupabase()
	}

	go ciStatsReconcileLoop()

	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/webhooks/github", handleGitHubWebhook) // HMAC-authenticated, see ci_stats_handlers.go
	http.HandleFunc("/auth/oidc/start", handleOIDCStart)
	http.HandleFunc("/auth/oidc/callback", handleOIDCCallback)
	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/sudo/toggle", requireAuth(handleSudoToggle))
	http.HandleFunc("/", requireAuth(handleHome))
	http.HandleFunc("/dashboard", requireAuth(handleDashboard))
	http.HandleFunc("/agent-workspaces", requireAuth(handleAgentWorkspaces))
	http.HandleFunc("/workspaces", requireAuth(handleManagedWorkspaces))
	http.HandleFunc("/api/workspaces", requireAuth(handleAPIManagedWorkspaces))
	http.HandleFunc("/api/stats", requireAuth(handleAPIStats))
	http.HandleFunc("/services", requireAuth(handleServices))
	http.HandleFunc("/api/services", requireAuth(handleAPIServices))
	http.HandleFunc("/api/services/action", requireAuth(handleServiceAction))
	http.HandleFunc("/packages", requireAuth(handlePackages))
	http.HandleFunc("/api/packages/updates", requireAuth(handleAPIPackageUpdates))
	http.HandleFunc("/api/packages/action", requireAuth(handlePackageAction))
	http.HandleFunc("/docker", requireAuth(handleDocker))
	http.HandleFunc("/api/docker/action", requireAuth(handleDockerAction))
	http.HandleFunc("/files", requireAuth(handleFileManager))
	http.HandleFunc("/api/files", requireAuth(handleFileAction))
	http.HandleFunc("/api/files/download", requireAuth(handleFileDownload))
	http.HandleFunc("/system", requireAuth(handleSystemActions))
	http.HandleFunc("/api/system/action", requireAuth(handleSystemActionAPI))
	http.HandleFunc("/logs", requireAuth(handleLogs))
	http.HandleFunc("/api/logs", requireAuth(handleAPILogs))
	http.HandleFunc("/cron", requireAuth(handleCron))
	http.HandleFunc("/api/cron", requireAuth(handleAPICron))
	http.HandleFunc("/users", requireAuth(handleUsers))
	http.HandleFunc("/api/users", requireAuth(handleAPIUsers))
	http.HandleFunc("/firewall", requireAuth(handleFirewall))
	http.HandleFunc("/api/firewall", requireAuth(handleAPIFirewall))
	http.HandleFunc("/sshkeys", requireAuth(handleSSHKeys))
	http.HandleFunc("/api/sshkeys", requireAuth(handleAPISSHKeys))
	http.HandleFunc("/dns", requireAuth(handleDNS))
	http.HandleFunc("/api/dns", requireAuth(handleAPIDNS))
	http.HandleFunc("/endpoints", requireAuth(handleEndpoints))
	http.HandleFunc("/api/endpoints", requireAuth(handleAPIEndpoints))
	http.HandleFunc("/terminal", requireAuth(handleTerminalPage))
	http.HandleFunc("/ws/terminal", requireAuth(handleTerminalWS))
	http.HandleFunc("/sysinfo", requireAuth(handleSysInfo))
	http.HandleFunc("/config", requireAuth(handleConfig))
	http.HandleFunc("/api/config", requireAuth(handleAPIConfig))
	http.HandleFunc("/help", requireAuth(handleHelp))
	http.HandleFunc("/audit", requireAuth(handleAudit))
	http.HandleFunc("/api/audit", requireAuth(handleAPIAudit))
	http.HandleFunc("/certs", requireAuth(handleCerts))
	http.HandleFunc("/api/certs", requireAuth(handleAPICerts))
	http.HandleFunc("/disks", requireAuth(handleDisks))
	http.HandleFunc("/api/disks", requireAuth(handleAPIDisks))
	http.HandleFunc("/proxmox", requireAuth(handleProxmox))
	http.HandleFunc("/api/proxmox", requireAuth(handleAPIProxmox))
	http.HandleFunc("/api/infrastructure", requireAuth(handleAPIInfrastructure))
	http.HandleFunc("/api/infrastructure/", requireAuth(handleAPIInfrastructure))
	http.HandleFunc("/catalog", requireAuth(handleCatalog))
	http.HandleFunc("/api/catalog", requireAuth(handleAPICatalog))
	http.HandleFunc("/api/catalog/", requireAuth(handleAPICatalog))
	http.HandleFunc("/deployments", requireAuth(handleDeployments))
	http.HandleFunc("/customers", requireAuth(handleCustomersIndex))
	http.HandleFunc("/customers/", requireAuth(handleCustomerDrill))
	http.HandleFunc("/internal", requireAuth(handleInternalDeployments))
	http.HandleFunc("/deployments/new", requireAuth(handleDeploymentsNew))
	http.HandleFunc("/deployments/", requireAuth(handleDeploymentDetail))
	http.HandleFunc("/api/deployments", requireAuth(handleAPIDeployments))
	http.HandleFunc("/api/deployments/", requireAuth(handleAPIDeployments))
	http.HandleFunc("/network", requireAuth(handleNetwork))
	http.HandleFunc("/api/network", requireAuth(handleAPINetwork))
	http.HandleFunc("/notifications", requireAuth(handleNotifications))
	http.HandleFunc("/api/notifications", requireAuth(handleAPINotifications))
	http.HandleFunc("/backups", requireAuth(handleBackups))
	http.HandleFunc("/api/backups", requireAuth(handleAPIBackups))
	http.HandleFunc("/api/backups/", requireAuth(handleAPIBackups))
	http.HandleFunc("/api/platform-systems", requireAuth(handleAPIPlatformSystems))
	http.HandleFunc("/runners", requireAuth(handleRunners))
	http.HandleFunc("/api/runners", requireAuth(handleAPIRunners))
	http.HandleFunc("/api/ci/stats", requireAuth(handleAPICIStats))
	http.HandleFunc("/api/version", requireAuth(handleAPIVersion))
	http.Handle("/static/", http.FileServer(http.FS(staticFS)))

	if tlsEnabled {
		if tlsCert != "" && tlsKey != "" {
			log.Printf("Warden v%s starting on https://localhost:%s (TLS with provided cert)", Version, port)
			log.Fatal(http.ListenAndServeTLS(":"+port, tlsCert, tlsKey, nil))
		} else {
			log.Printf("Warden v%s starting on https://localhost:%s (TLS with auto-generated self-signed cert)", Version, port)
			tlsConfig, err := generateSelfSignedTLS()
			if err != nil {
				log.Fatalf("Failed to generate self-signed certificate: %v", err)
			}
			server := &http.Server{
				Addr:      ":" + port,
				TLSConfig: tlsConfig,
			}
			log.Fatal(server.ListenAndServeTLS("", ""))
		}
	} else {
		log.Printf("Warden v%s starting on http://localhost:%s", Version, port)
		log.Fatal(http.ListenAndServe(":"+port, nil))
	}
}

func generateSelfSignedTLS() (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Warden"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats := collectStats()
	stats.Username, _ = getSession(r)
	stats.SudoEnabled = isSudoEnabled(r)
	stats.Version = getVersionInfo()
	stats.UptimeFmt = formatUptime(stats.Uptime)
	stats.PageTitle = "Dashboard"
	stats.HealthWidgets = buildHealthWidgets(stats)
	stats.HostNodes = buildHostNodes(stats)
	stats.DashboardTabs = buildDashboardTabs(stats.HostNodes)
	stats.MainCharts = buildChartPerspectives(stats, stats.HostNodes)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "index.html", stats); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIStats(w http.ResponseWriter, r *http.Request) {
	stats := collectStats()
	stats.Version = getVersionInfo()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func handleAPIVersion(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("check") == "now" {
		checkLatestVersion()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(getVersionInfo())
}

func handleServices(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	units := getSystemdUnits(filter)
	hd := buildHeaderData(r)
	hd.PageTitle = "Services"
	data := struct {
		HeaderData
		Units   []SystemdUnit
		Filter  string
		Message string
	}{
		HeaderData: hd,
		Units:      units,
		Filter:     filter,
		Message:    r.URL.Query().Get("msg"),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "services.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIServices(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	units := getSystemdUnits(filter)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(units)
}

func handleServiceAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.ParseForm()
	action := r.FormValue("action")
	unit := r.FormValue("unit")

	if unit == "" || action == "" {
		http.Error(w, "Missing unit or action", http.StatusBadRequest)
		return
	}

	// Validate action
	validActions := map[string]bool{
		"start": true, "stop": true, "restart": true,
		"enable": true, "disable": true,
	}
	if !validActions[action] {
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	// Validate unit name (alphanumeric, dash, dot, underscore, @)
	for _, c := range unit {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '@') {
			http.Error(w, "Invalid unit name", http.StatusBadRequest)
			return
		}
	}

	var out string
	var err error
	if isSudoEnabled(r) {
		_, password, _ := getSessionPassword(r)
		out, err = sudoExecCommand(password, "systemctl", action, unit)
	} else {
		out, err = execCommand("systemctl", action, unit)
	}
	msg := action + " " + unit + ": success"
	if err != nil {
		msg = action + " " + unit + ": " + strings.TrimSpace(out)
		if msg == action+" "+unit+": " {
			msg = action + " " + unit + ": failed"
		}
	}

	if err == nil {
		auditFromRequest(r, "Service "+action, unit, "service")
	}

	// If API request, return JSON
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": err == nil,
			"message": msg,
		})
		return
	}

	// Otherwise redirect back to services page
	http.Redirect(w, r, "/services?msg="+msg, http.StatusSeeOther)
}

func getSystemdUnits(filter string) []SystemdUnit {
	args := []string{"list-units", "--all", "--no-pager", "--no-legend",
		"--plain", "-o", "json"}
	if filter != "" {
		args = append(args, "*"+filter+"*")
	} else {
		args = append(args, "--type=service")
	}

	out, err := execCommand("systemctl", args...)
	if err != nil {
		// Fallback to text parsing if JSON not supported
		return getSystemdUnitsText(filter)
	}

	// Try JSON parse
	var jsonUnits []struct {
		Unit        string `json:"unit"`
		Load        string `json:"load"`
		Active      string `json:"active"`
		Sub         string `json:"sub"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(out), &jsonUnits); err != nil {
		return getSystemdUnitsText(filter)
	}

	// Batch fetch enabled states
	var names []string
	for _, ju := range jsonUnits {
		names = append(names, ju.Unit)
	}
	enabledStates := getEnabledStates(names)

	var units []SystemdUnit
	for _, ju := range jsonUnits {
		unitType := "service"
		if idx := strings.LastIndex(ju.Unit, "."); idx >= 0 {
			unitType = ju.Unit[idx+1:]
		}

		units = append(units, SystemdUnit{
			Name:        ju.Unit,
			Description: ju.Description,
			UnitType:    unitType,
			ActiveState: ju.Active,
			SubState:    ju.Sub,
			Enabled:     enabledStates[ju.Unit],
			Running:     ju.Sub == "running",
		})
	}

	return units
}

func getSystemdUnitsText(filter string) []SystemdUnit {
	args := []string{"list-units", "--all", "--no-pager", "--no-legend", "--plain"}
	if filter != "" {
		args = append(args, "*"+filter+"*")
	} else {
		args = append(args, "--type=service")
	}

	out, err := execCommand("systemctl", args...)
	if err != nil {
		return nil
	}

	// First pass: collect names
	var names []string
	type rawUnit struct {
		name, active, sub, desc, unitType string
	}
	var raw []rawUnit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		name := fields[0]
		unitType := "service"
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			unitType = name[idx+1:]
		}
		names = append(names, name)
		raw = append(raw, rawUnit{name, fields[2], fields[3], strings.Join(fields[4:], " "), unitType})
	}

	enabledStates := getEnabledStates(names)

	var units []SystemdUnit
	for _, r := range raw {
		units = append(units, SystemdUnit{
			Name:        r.name,
			Description: r.desc,
			UnitType:    r.unitType,
			ActiveState: r.active,
			SubState:    r.sub,
			Enabled:     enabledStates[r.name],
			Running:     r.sub == "running",
		})
	}

	return units
}

func getEnabledStates(units []string) map[string]string {
	if len(units) == 0 {
		return nil
	}
	args := append([]string{"is-enabled", "--"}, units...)
	out, _ := execCommand("systemctl", args...)
	states := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i, line := range lines {
		if i < len(units) {
			states[units[i]] = strings.TrimSpace(line)
		}
	}
	return states
}

func collectStats() SystemStats {
	stats := SystemStats{
		CollectedAt: time.Now(),
		Arch:        runtime.GOARCH,
		GoVersion:   runtime.Version(),
	}

	stats.Hostname, _ = os.Hostname()
	stats.OS = readFileString("/etc/os-release", "PRETTY_NAME")
	stats.Kernel = readFileTrimmed("/proc/version")
	if idx := strings.Index(stats.Kernel, "("); idx > 0 {
		stats.Kernel = strings.TrimSpace(stats.Kernel[:idx])
	}
	stats.Uptime = getUptime()
	stats.LoadAvg = getLoadAvg()
	stats.CPU = getCPUInfo()
	stats.Memory = getMemInfo("MemTotal", "MemFree", "MemAvailable", "Buffers", "Cached")
	stats.Swap = getSwapInfo()
	stats.Disks = getDiskInfo()
	stats.Network = getNetInfo()
	stats.TopProcs = getTopProcesses(15)
	stats.LoggedIn = getLoggedInUsers()
	stats.Temps = getTemperatures()
	stats.Listeners = getListeningPorts()
	stats.DiskIO = getDiskIOStats()
	stats.RecentLogins = getRecentLogins()
	stats.ProcessCount = getProcessCount()
	stats.PackageUpdates = getPackageUpdates()

	return stats
}

// --- Readers ---

func readFileTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "N/A"
	}
	return strings.TrimSpace(string(data))
}

func readFileString(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "N/A"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, key+"=") {
			val := strings.TrimPrefix(line, key+"=")
			return strings.Trim(val, "\"")
		}
	}
	return "N/A"
}

func parseUint(s string) uint64 {
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

// --- System Stats ---

func getUptime() time.Duration {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	parts := strings.Fields(string(data))
	if len(parts) < 1 {
		return 0
	}
	secs, _ := strconv.ParseFloat(parts[0], 64)
	return time.Duration(secs * float64(time.Second))
}

func getLoadAvg() LoadAverage {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return LoadAverage{"N/A", "N/A", "N/A"}
	}
	parts := strings.Fields(string(data))
	if len(parts) < 3 {
		return LoadAverage{"N/A", "N/A", "N/A"}
	}
	return LoadAverage{parts[0], parts[1], parts[2]}
}

func getCPUInfo() CPUInfo {
	info := CPUInfo{Cores: runtime.NumCPU()}

	data, err := os.ReadFile("/proc/cpuinfo")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					info.ModelName = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	// Read CPU usage from /proc/stat (snapshot-based)
	stat1 := readCPUStat()
	time.Sleep(200 * time.Millisecond)
	stat2 := readCPUStat()

	if len(stat1) >= 4 && len(stat2) >= 4 {
		user := float64(stat2[0] - stat1[0])
		nice := float64(stat2[1] - stat1[1])
		system := float64(stat2[2] - stat1[2])
		idle := float64(stat2[3] - stat1[3])
		total := user + nice + system + idle

		if total > 0 {
			info.UserPerc = (user + nice) / total * 100
			info.SystemPerc = system / total * 100
			info.IdlePerc = idle / total * 100
			info.UsagePerc = 100 - info.IdlePerc
		}
	}

	return info
}

func readCPUStat() []uint64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return nil
	}
	fields := strings.Fields(lines[0]) // "cpu" line
	if len(fields) < 5 {
		return nil
	}
	var vals []uint64
	for _, f := range fields[1:] {
		v, _ := strconv.ParseUint(f, 10, 64)
		vals = append(vals, v)
	}
	return vals
}

func getMemInfo(keys ...string) MemInfo {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemInfo{}
	}

	vals := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		for _, key := range keys {
			if strings.HasPrefix(line, key+":") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					vals[key] = parseUint(fields[1]) * 1024 // kB to bytes
				}
			}
		}
	}

	total := vals["MemTotal"]
	free := vals["MemFree"]
	available := vals["MemAvailable"]
	buffers := vals["Buffers"]
	cached := vals["Cached"]
	used := total - free - buffers - cached

	var usedPerc float64
	if total > 0 {
		usedPerc = float64(used) / float64(total) * 100
	}

	return MemInfo{
		Total:     total,
		Used:      used,
		Free:      free,
		Available: available,
		UsedPerc:  usedPerc,
	}
}

func getSwapInfo() MemInfo {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemInfo{}
	}

	vals := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		for _, key := range []string{"SwapTotal", "SwapFree"} {
			if strings.HasPrefix(line, key+":") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					vals[key] = parseUint(fields[1]) * 1024
				}
			}
		}
	}

	total := vals["SwapTotal"]
	free := vals["SwapFree"]
	used := total - free

	var usedPerc float64
	if total > 0 {
		usedPerc = float64(used) / float64(total) * 100
	}

	return MemInfo{Total: total, Used: used, Free: free, UsedPerc: usedPerc}
}

func getDiskInfo() []DiskInfo {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil
	}

	var disks []DiskInfo
	seen := make(map[string]bool)

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		device, mount, fstype := fields[0], fields[1], fields[2]

		// Skip virtual filesystems and squashfs (snap packages)
		if !strings.HasPrefix(device, "/dev/") {
			continue
		}
		if fstype == "squashfs" {
			continue
		}
		if seen[device] {
			continue
		}
		seen[device] = true

		var stat syscallStatfs
		if err := statfs(mount, &stat); err != nil {
			continue
		}

		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bavail * uint64(stat.Bsize)
		used := total - free
		var usedPerc float64
		if total > 0 {
			usedPerc = float64(used) / float64(total) * 100
		}

		disks = append(disks, DiskInfo{
			Device:     device,
			MountPoint: mount,
			FSType:     fstype,
			Total:      total,
			Used:       used,
			Free:       free,
			UsedPerc:   usedPerc,
		})
	}

	return disks
}

func getNetInfo() []NetInfo {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil
	}

	var nets []NetInfo
	lines := strings.Split(string(data), "\n")

	for _, line := range lines[2:] { // skip header lines
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colonIdx])
		if name == "lo" {
			continue
		}
		// Skip Docker/container-related interfaces
		if strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") ||
			strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "flannel") {
			continue
		}

		rest := strings.Fields(line[colonIdx+1:])
		if len(rest) < 10 {
			continue
		}

		ni := NetInfo{
			Name:    name,
			RxBytes: parseUint(rest[0]),
			TxBytes: parseUint(rest[8]),
			IPv4:    getIPv4(name),
			State:   getIfState(name),
		}
		nets = append(nets, ni)
	}

	return nets
}

func getIPv4(iface string) string {
	// Read from /sys/class/net or fallback
	data, err := os.ReadFile(fmt.Sprintf("/proc/net/fib_trie"))
	if err != nil {
		return "N/A"
	}
	// Simple approach: read ip command output would be better, but let's parse /proc/net/if_inet6 alternative
	// Actually let's just use a simpler approach via /sys
	_ = data

	// Read from ip addr - but we want to avoid exec. Let's parse /proc/net/fib_trie
	// Simpler: just indicate we need the ip tool, or parse from RTNETLINK
	// For simplicity, we'll use a small exec
	return getIPFromProc(iface)
}

func getIPFromProc(iface string) string {
	// Parse /proc/net/fib_trie is complex; use /sys/class/net approach
	// Actually the cleanest no-exec way is reading from netlink, but that's complex
	// Let's use os/exec for this one thing
	out, err := execCommand("ip", "-4", "-o", "addr", "show", iface)
	if err != nil || out == "" {
		return "N/A"
	}
	fields := strings.Fields(out)
	for i, f := range fields {
		if f == "inet" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return "N/A"
}

func getIfState(iface string) string {
	state := readFileTrimmed(fmt.Sprintf("/sys/class/net/%s/operstate", iface))
	return state
}

func getTopProcesses(n int) []ProcInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	var procs []ProcInfo
	pageSize := uint64(os.Getpagesize())
	totalMem := getTotalMemKB() * 1024

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		stat := readFileTrimmed(fmt.Sprintf("/proc/%d/stat", pid))
		if stat == "N/A" || stat == "" {
			continue
		}

		// Parse comm (between parentheses) to handle spaces in command names
		openParen := strings.Index(stat, "(")
		closeParen := strings.LastIndex(stat, ")")
		if openParen < 0 || closeParen < 0 {
			continue
		}
		comm := stat[openParen+1 : closeParen]
		rest := strings.Fields(stat[closeParen+2:])
		if len(rest) < 21 {
			continue
		}

		utime, _ := strconv.ParseUint(rest[11], 10, 64)
		stime, _ := strconv.ParseUint(rest[12], 10, 64)
		rss, _ := strconv.ParseUint(rest[21], 10, 64)
		rssBytes := rss * pageSize

		var memPerc float64
		if totalMem > 0 {
			memPerc = float64(rssBytes) / float64(totalMem) * 100
		}

		// Get username from /proc/pid/status
		user := getProcUser(pid)

		// Read cmdline for full command
		cmdline := readFileTrimmed(fmt.Sprintf("/proc/%d/cmdline", pid))
		if cmdline == "N/A" || cmdline == "" {
			cmdline = "[" + comm + "]"
		} else {
			cmdline = strings.ReplaceAll(cmdline, "\x00", " ")
			cmdline = strings.TrimSpace(cmdline)
			if len(cmdline) > 80 {
				cmdline = cmdline[:80] + "..."
			}
		}

		procs = append(procs, ProcInfo{
			PID:     pid,
			User:    user,
			CPU:     float64(utime + stime),
			Mem:     memPerc,
			Command: cmdline,
		})
	}

	// Sort by CPU time descending
	sort.Slice(procs, func(i, j int) bool {
		return procs[i].CPU > procs[j].CPU
	})

	if len(procs) > n {
		procs = procs[:n]
	}

	return procs
}

func getProcUser(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return "?"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				uid := fields[1]
				return uidToName(uid)
			}
		}
	}
	return "?"
}

func uidToName(uid string) string {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return uid
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) >= 3 && parts[2] == uid {
			return parts[0]
		}
	}
	return uid
}

func getTotalMemKB() uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return parseUint(fields[1])
			}
		}
	}
	return 0
}

// --- Logged-in Users ---

func getLoggedInUsers() []LoggedInUser {
	out, err := execCommand("who")
	if err != nil {
		return nil
	}
	var users []LoggedInUser
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		u := LoggedInUser{
			User:     fields[0],
			Terminal: fields[1],
			LoginAt:  fields[2],
		}
		if len(fields) >= 4 {
			u.LoginAt += " " + fields[3]
		}
		if len(fields) >= 5 {
			u.From = strings.Trim(fields[4], "()")
		}
		users = append(users, u)
	}
	return users
}

// --- Temperatures ---

func getTemperatures() []TempSensor {
	var sensors []TempSensor

	// Read from hwmon for detailed sensor data
	hwmonBase := "/sys/class/hwmon"
	hwmonEntries, err := os.ReadDir(hwmonBase)
	if err == nil {
		for _, entry := range hwmonEntries {
			dir := hwmonBase + "/" + entry.Name()
			chipName := readFileTrimmed(dir + "/name")
			// Look for temp*_input files
			files, _ := os.ReadDir(dir)
			for _, f := range files {
				if !strings.HasPrefix(f.Name(), "temp") || !strings.HasSuffix(f.Name(), "_input") {
					continue
				}
				tempStr := readFileTrimmed(dir + "/" + f.Name())
				if tempStr == "N/A" {
					continue
				}
				tempMilli, _ := strconv.ParseFloat(tempStr, 64)
				// Try to get label
				labelFile := strings.Replace(f.Name(), "_input", "_label", 1)
				label := readFileTrimmed(dir + "/" + labelFile)
				if label == "N/A" {
					label = chipName + " " + strings.TrimSuffix(strings.TrimPrefix(f.Name(), "temp"), "_input")
				} else {
					label = chipName + ": " + label
				}
				temp := tempMilli / 1000.0
				if temp <= 0 || temp > 120 {
					continue
				}
				sensors = append(sensors, TempSensor{
					Label: label,
					Temp:  temp,
				})
			}
		}
	}

	// Fallback to thermal_zone if hwmon found nothing
	if len(sensors) == 0 {
		entries, err := os.ReadDir("/sys/class/thermal")
		if err == nil {
			for _, entry := range entries {
				if !strings.HasPrefix(entry.Name(), "thermal_zone") {
					continue
				}
				base := "/sys/class/thermal/" + entry.Name()
				typeStr := readFileTrimmed(base + "/type")
				tempStr := readFileTrimmed(base + "/temp")
				if tempStr == "N/A" {
					continue
				}
				tempMilli, _ := strconv.ParseFloat(tempStr, 64)
				sensors = append(sensors, TempSensor{
					Label: typeStr,
					Temp:  tempMilli / 1000.0,
				})
			}
		}
	}

	return sensors
}

// --- Listening Ports ---

func getListeningPorts() []ListenPort {
	out, err := execCommand("ss", "-tulnp")
	if err != nil {
		return nil
	}

	var ports []ListenPort
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if fields[1] != "LISTEN" && fields[1] != "UNCONN" {
			continue
		}

		proto := fields[0]
		localAddr := fields[4]

		// Split address and port
		lastColon := strings.LastIndex(localAddr, ":")
		if lastColon < 0 {
			continue
		}
		addr := localAddr[:lastColon]
		portNum, _ := strconv.Atoi(localAddr[lastColon+1:])

		// Skip Docker bridge addresses (172.17-31.x.x)
		if isDockerAddr(addr) {
			continue
		}

		// Parse process info from the last field like "users:(("sshd",pid=1234,fd=3))"
		procName := ""
		pid := 0
		for _, f := range fields[5:] {
			if strings.HasPrefix(f, "users:") {
				if start := strings.Index(f, "((\""); start >= 0 {
					end := strings.Index(f[start+3:], "\"")
					if end >= 0 {
						procName = f[start+3 : start+3+end]
					}
				}
				if pidIdx := strings.Index(f, "pid="); pidIdx >= 0 {
					rest := f[pidIdx+4:]
					if comma := strings.IndexAny(rest, ",)"); comma >= 0 {
						pid, _ = strconv.Atoi(rest[:comma])
					}
				}
			}
		}

		ports = append(ports, ListenPort{
			Proto:   proto,
			Address: addr,
			Port:    portNum,
			PID:     pid,
			Process: procName,
		})
	}

	// Filter out ephemeral ports (>32767) without a known process
	var filtered []ListenPort
	for _, p := range ports {
		if p.Port > 32767 && p.Process == "" {
			continue
		}
		filtered = append(filtered, p)
	}
	ports = filtered

	// Deduplicate by port — prefer wildcard addresses (0.0.0.0, ::, *)
	portMap := make(map[int]ListenPort)
	for _, p := range ports {
		existing, ok := portMap[p.Port]
		if !ok {
			portMap[p.Port] = p
			continue
		}
		// Prefer wildcard addresses
		if p.Address == "0.0.0.0" || p.Address == "::" || p.Address == "*" {
			portMap[p.Port] = p
		} else if existing.Process == "" && p.Process != "" {
			portMap[p.Port] = p
		}
	}
	var unique []ListenPort
	for _, p := range portMap {
		unique = append(unique, p)
	}

	sort.Slice(unique, func(i, j int) bool {
		return unique[i].Port < unique[j].Port
	})

	return unique
}

func isDockerAddr(addr string) bool {
	// Docker typically uses 172.17.0.0/16 through 172.31.0.0/16
	if !strings.HasPrefix(addr, "172.") {
		return false
	}
	parts := strings.SplitN(addr, ".", 3)
	if len(parts) < 2 {
		return false
	}
	second, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return second >= 17 && second <= 31
}

// --- Disk I/O ---

func getDiskIOStats() []DiskIOInfo {
	data, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return nil
	}

	var stats []DiskIOInfo
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}
		devName := fields[2]

		// Skip partitions (only show whole disks) and loop/dm devices
		if strings.HasPrefix(devName, "loop") || strings.HasPrefix(devName, "dm-") {
			continue
		}
		// Skip partitions: if it ends with a digit and has a letter before it, it's likely a partition
		// But keep nvme partitions like nvme0n1 (not nvme0n1p1)
		if isPartition(devName) {
			continue
		}

		readsCompleted := parseUint(fields[3])
		readSectors := parseUint(fields[5])
		writesCompleted := parseUint(fields[7])
		writeSectors := parseUint(fields[9])

		stats = append(stats, DiskIOInfo{
			Device:          devName,
			ReadsCompleted:  readsCompleted,
			WritesCompleted: writesCompleted,
			ReadBytes:       readSectors * 512,
			WriteBytes:      writeSectors * 512,
		})
	}

	return stats
}

func isPartition(name string) bool {
	// sda1, sdb2, etc.
	if len(name) >= 4 && name[0] == 's' && name[1] == 'd' {
		last := name[len(name)-1]
		return last >= '0' && last <= '9'
	}
	// nvme0n1p1
	if strings.HasPrefix(name, "nvme") && strings.Contains(name, "p") {
		parts := strings.Split(name, "p")
		if len(parts) > 1 {
			_, err := strconv.Atoi(parts[len(parts)-1])
			return err == nil
		}
	}
	return false
}

// --- Recent Logins ---

func getRecentLogins() []RecentLogin {
	var logins []RecentLogin

	// Recent successful logins
	out, err := execCommand("last", "-n", "5", "-w")
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" || strings.HasPrefix(line, "wtmp") || strings.HasPrefix(line, "reboot") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			timeStr := strings.Join(fields[2:], " ")
			if len(timeStr) > 30 {
				timeStr = timeStr[:30]
			}
			from := ""
			if len(fields) >= 3 && !strings.HasPrefix(fields[2], "Mon") &&
				!strings.HasPrefix(fields[2], "Tue") && !strings.HasPrefix(fields[2], "Wed") &&
				!strings.HasPrefix(fields[2], "Thu") && !strings.HasPrefix(fields[2], "Fri") &&
				!strings.HasPrefix(fields[2], "Sat") && !strings.HasPrefix(fields[2], "Sun") {
				from = fields[2]
				timeStr = strings.Join(fields[3:], " ")
				if len(timeStr) > 30 {
					timeStr = timeStr[:30]
				}
			}
			logins = append(logins, RecentLogin{
				User:    fields[0],
				From:    from,
				Time:    timeStr,
				Success: true,
			})
		}
	}

	// Recent failed SSH attempts from auth log
	authLog := "/var/log/auth.log"
	if _, err := os.Stat(authLog); os.IsNotExist(err) {
		authLog = "/var/log/secure" // RHEL/CentOS
	}
	data, err := os.ReadFile(authLog)
	if err == nil {
		lines := strings.Split(string(data), "\n")
		count := 0
		for i := len(lines) - 1; i >= 0 && count < 5; i-- {
			line := lines[i]
			if !strings.Contains(line, "Failed password") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			timeStr := strings.Join(parts[0:3], " ")
			user := ""
			from := ""
			for j, p := range parts {
				if p == "user" && j+1 < len(parts) {
					user = parts[j+1]
				}
				if p == "from" && j+1 < len(parts) {
					from = parts[j+1]
				}
			}
			logins = append(logins, RecentLogin{
				User:    user,
				From:    from,
				Time:    timeStr,
				Success: false,
			})
			count++
		}
	}

	return logins
}

// --- Process Count ---

func getProcessCount() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err == nil {
			count++
		}
	}
	return count
}

// --- Package Updates ---

func getPackageUpdates() PackageInfo {
	var info PackageInfo

	// Try apt (Debian/Ubuntu)
	if _, err := os.Stat("/usr/bin/apt"); err == nil {
		out, err := execCommand("apt", "list", "--upgradable")
		if err == nil {
			lines := strings.Split(strings.TrimSpace(out), "\n")
			for _, line := range lines {
				if strings.Contains(line, "upgradable") || strings.Contains(line, "/") {
					if strings.Contains(line, "Listing...") {
						continue
					}
					info.Total++
					if strings.Contains(line, "-security") {
						info.Security++
					}
				}
			}
		}
		return info
	}

	// Try dnf (RHEL/Fedora)
	if _, err := os.Stat("/usr/bin/dnf"); err == nil {
		out, err := execCommand("dnf", "check-update", "--quiet")
		if err == nil || strings.TrimSpace(out) != "" {
			lines := strings.Split(strings.TrimSpace(out), "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "Last metadata") {
					info.Total++
				}
			}
		}
		return info
	}

	return info
}

// --- Formatting ---

func levelClass(pct float64) string {
	switch {
	case pct >= 90:
		return "level-crit"
	case pct >= 75:
		return "level-high"
	case pct >= 50:
		return "level-warn"
	default:
		return "level-ok"
	}
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), []string{"KB", "MB", "GB", "TB", "PB"}[exp])
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}
