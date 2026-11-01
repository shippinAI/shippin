package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

type PackageEntry struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	NewVersion   string `json:"new_version,omitempty"`
	Architecture string `json:"architecture,omitempty"`
	Description  string `json:"description"`
	Size         string `json:"size,omitempty"`
	Status       string `json:"status"` // installed, upgradable, available
}

type pkgManager int

const (
	pkgNone pkgManager = iota
	pkgApt
	pkgDnf
	pkgPacman
)

func detectPkgManager() pkgManager {
	if _, err := os.Stat("/usr/bin/apt"); err == nil {
		return pkgApt
	}
	if _, err := os.Stat("/usr/bin/dnf"); err == nil {
		return pkgDnf
	}
	if _, err := os.Stat("/usr/bin/pacman"); err == nil {
		return pkgPacman
	}
	return pkgNone
}

func validatePackageName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '+' || c == ':') {
			return false
		}
	}
	return true
}

func getInstalledPackages(search string, pm pkgManager) []PackageEntry {
	var out string
	var err error

	switch pm {
	case pkgApt:
		out, err = execCommand("dpkg-query", "-W", "-f=${Package}\t${Version}\t${Architecture}\t${Installed-Size}\t${Description}\n")
	case pkgDnf:
		out, err = execCommand("rpm", "-qa", "--queryformat", "%{NAME}\t%{VERSION}-%{RELEASE}\t%{ARCH}\t%{SIZE}\t%{SUMMARY}\n")
	case pkgPacman:
		out, err = execCommand("pacman", "-Q")
		if err != nil {
			return nil
		}
		// pacman -Q outputs: name version
		searchLower := strings.ToLower(search)
		var pkgs []PackageEntry
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			name := fields[0]
			if search != "" && !strings.Contains(strings.ToLower(name), searchLower) {
				continue
			}
			// Get details with pacman -Qi for matching packages
			pkgs = append(pkgs, PackageEntry{
				Name:    name,
				Version: fields[1],
				Status:  "installed",
			})
		}
		// Enrich with size/description for filtered results
		if search != "" && len(pkgs) <= 100 {
			for i := range pkgs {
				info, e := execCommand("pacman", "-Qi", pkgs[i].Name)
				if e == nil {
					pkgs[i].Description = parsePacmanField(info, "Description")
					pkgs[i].Size = parsePacmanField(info, "Installed Size")
					pkgs[i].Architecture = parsePacmanField(info, "Architecture")
				}
			}
		}
		if len(pkgs) > 500 {
			pkgs = pkgs[:500]
		}
		return pkgs
	default:
		return nil
	}
	if err != nil {
		return nil
	}

	searchLower := strings.ToLower(search)
	var pkgs []PackageEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 5)
		if len(fields) < 3 {
			continue
		}

		name := fields[0]
		if search != "" && !strings.Contains(strings.ToLower(name), searchLower) &&
			(len(fields) < 5 || !strings.Contains(strings.ToLower(fields[4]), searchLower)) {
			continue
		}

		p := PackageEntry{
			Name:         name,
			Version:      fields[1],
			Architecture: fields[2],
			Status:       "installed",
		}
		if len(fields) >= 4 {
			size := fields[3]
			if pm == pkgApt {
				p.Size = size + " kB"
			} else {
				// rpm returns bytes
				s := parseUint(size)
				p.Size = formatBytes(s)
			}
		}
		if len(fields) >= 5 {
			desc := fields[4]
			// dpkg includes multi-line; take first line
			if idx := strings.Index(desc, "\n"); idx >= 0 {
				desc = desc[:idx]
			}
			if len(desc) > 100 {
				desc = desc[:100] + "..."
			}
			p.Description = desc
		}
		pkgs = append(pkgs, p)
	}

	// Cap results
	if len(pkgs) > 500 {
		pkgs = pkgs[:500]
	}
	return pkgs
}

func getUpdatablePackages(pm pkgManager) []PackageEntry {
	var pkgs []PackageEntry

	switch pm {
	case pkgApt:
		out, err := execCommand("apt", "list", "--upgradable")
		if err != nil {
			return nil
		}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" || strings.HasPrefix(line, "Listing") {
				continue
			}
			// Format: name/source version arch [upgradable from: oldversion]
			parts := strings.SplitN(line, "/", 2)
			if len(parts) < 2 {
				continue
			}
			name := parts[0]
			rest := parts[1]
			fields := strings.Fields(rest)
			newVersion := ""
			arch := ""
			oldVersion := ""
			if len(fields) >= 1 {
				// Skip source, next is version
				for i, f := range fields {
					if i == 0 {
						continue // source
					}
					if newVersion == "" {
						newVersion = f
						continue
					}
					if arch == "" {
						arch = f
						continue
					}
					if f == "from:" && i+1 < len(fields) {
						oldVersion = strings.TrimSuffix(fields[i+1], "]")
						break
					}
				}
			}
			pkgs = append(pkgs, PackageEntry{
				Name:         name,
				Version:      oldVersion,
				NewVersion:   newVersion,
				Architecture: arch,
				Status:       "upgradable",
			})
		}

	case pkgDnf:
		out, _ := execCommand("dnf", "check-update", "--quiet")
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "Last metadata") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			name := fields[0]
			// Remove arch suffix from name if present (e.g., package.x86_64)
			if idx := strings.LastIndex(name, "."); idx >= 0 {
				name = name[:idx]
			}
			pkgs = append(pkgs, PackageEntry{
				Name:       name,
				NewVersion: fields[1],
				Status:     "upgradable",
			})
		}

	case pkgPacman:
		// checkupdates is the safe way to check for updates on Arch
		out, err := execCommand("checkupdates")
		if err != nil {
			// checkupdates returns exit code 2 when no updates, try pacman -Qu as fallback
			out, _ = execCommand("pacman", "-Qu")
		}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" {
				continue
			}
			// Format: name oldversion -> newversion
			fields := strings.Fields(line)
			if len(fields) >= 4 && fields[2] == "->" {
				pkgs = append(pkgs, PackageEntry{
					Name:       fields[0],
					Version:    fields[1],
					NewVersion: fields[3],
					Status:     "upgradable",
				})
			} else if len(fields) >= 2 {
				// pacman -Qu: name newversion
				pkgs = append(pkgs, PackageEntry{
					Name:       fields[0],
					NewVersion: fields[1],
					Status:     "upgradable",
				})
			}
		}
	}

	return pkgs
}

func searchAvailablePackages(query string, pm pkgManager) []PackageEntry {
	if query == "" {
		return nil
	}

	var pkgs []PackageEntry

	switch pm {
	case pkgApt:
		out, err := execCommand("apt-cache", "search", query)
		if err != nil {
			return nil
		}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, " - ", 2)
			if len(parts) < 2 {
				continue
			}
			desc := parts[1]
			if len(desc) > 100 {
				desc = desc[:100] + "..."
			}
			pkgs = append(pkgs, PackageEntry{
				Name:        parts[0],
				Description: desc,
				Status:      "available",
			})
		}

	case pkgDnf:
		out, err := execCommand("dnf", "search", "--quiet", query)
		if err != nil {
			return nil
		}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" || strings.HasPrefix(line, "=") {
				continue
			}
			parts := strings.SplitN(line, " : ", 2)
			if len(parts) < 2 {
				continue
			}
			name := parts[0]
			// Remove arch suffix
			if idx := strings.LastIndex(name, "."); idx >= 0 {
				name = name[:idx]
			}
			desc := parts[1]
			if len(desc) > 100 {
				desc = desc[:100] + "..."
			}
			pkgs = append(pkgs, PackageEntry{
				Name:        strings.TrimSpace(name),
				Description: desc,
				Status:      "available",
			})
		}

	case pkgPacman:
		out, err := execCommand("pacman", "-Ss", query)
		if err != nil {
			return nil
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		for i := 0; i < len(lines); i++ {
			line := lines[i]
			if line == "" || line[0] == ' ' {
				continue
			}
			// Format: repo/name version [installed]
			//     Description line
			parts := strings.SplitN(line, "/", 2)
			if len(parts) < 2 {
				continue
			}
			fields := strings.Fields(parts[1])
			if len(fields) < 1 {
				continue
			}
			name := fields[0]
			desc := ""
			if i+1 < len(lines) && len(lines[i+1]) > 0 && lines[i+1][0] == ' ' {
				desc = strings.TrimSpace(lines[i+1])
				i++ // skip description line
			}
			if len(desc) > 100 {
				desc = desc[:100] + "..."
			}
			pkgs = append(pkgs, PackageEntry{
				Name:        name,
				Description: desc,
				Status:      "available",
			})
		}
	}

	if len(pkgs) > 200 {
		pkgs = pkgs[:200]
	}
	return pkgs
}

// --- Handlers ---

func handlePackages(w http.ResponseWriter, r *http.Request) {
	pm := detectPkgManager()

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "updates"
	}
	search := r.URL.Query().Get("search")
	query := r.URL.Query().Get("q")

	var installed []PackageEntry
	var updates []PackageEntry
	var searchResults []PackageEntry

	switch tab {
	case "installed":
		installed = getInstalledPackages(search, pm)
	case "updates":
		updates = getUpdatablePackages(pm)
	case "search":
		if query != "" {
			searchResults = searchAvailablePackages(query, pm)
		}
	}

	pmName := "Unknown"
	switch pm {
	case pkgApt:
		pmName = "APT (Debian/Ubuntu)"
	case pkgDnf:
		pmName = "DNF (Fedora/RHEL)"
	case pkgPacman:
		pmName = "Pacman (Arch)"
	}

	hd := buildHeaderData(r)
	hd.PageTitle = "Packages"
	data := struct {
		HeaderData
		Tab            string
		Search         string
		Query          string
		Installed      []PackageEntry
		Updates        []PackageEntry
		SearchResults  []PackageEntry
		PackageManager string
		Message        string
	}{
		HeaderData:     hd,
		Tab:            tab,
		Search:         search,
		Query:          query,
		Installed:      installed,
		Updates:        updates,
		SearchResults:  searchResults,
		PackageManager: pmName,
		Message:        r.URL.Query().Get("msg"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "packages.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIPackageUpdates(w http.ResponseWriter, r *http.Request) {
	pm := detectPkgManager()
	updates := getUpdatablePackages(pm)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updates)
}

// pkgActionMu prevents concurrent package manager operations.
var pkgActionMu sync.Mutex

func handlePackageAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !pkgActionMu.TryLock() {
		log.Printf("[pkg] rejected concurrent package action")
		if r.Header.Get("Accept") == "application/json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "A package operation is already in progress. Please wait.",
			})
			return
		}
		http.Redirect(w, r, "/packages?msg=A package operation is already running", http.StatusSeeOther)
		return
	}
	defer pkgActionMu.Unlock()

	r.ParseForm()
	action := r.FormValue("action")
	pkg := r.FormValue("package")

	validActions := map[string]bool{
		"install": true, "remove": true, "upgrade": true, "upgrade-all": true,
	}
	if !validActions[action] {
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	if action != "upgrade-all" && !validatePackageName(pkg) {
		http.Error(w, "Invalid package name", http.StatusBadRequest)
		return
	}

	pm := detectPkgManager()
	if pm == pkgNone {
		http.Error(w, "No package manager found", http.StatusInternalServerError)
		return
	}

	var args []string
	switch pm {
	case pkgApt:
		switch action {
		case "install":
			args = []string{"apt-get", "install", "-y", pkg}
		case "remove":
			args = []string{"apt-get", "remove", "-y", pkg}
		case "upgrade":
			args = []string{"apt-get", "install", "--only-upgrade", "-y", pkg}
		case "upgrade-all":
			args = []string{"apt-get", "upgrade", "-y"}
		}
	case pkgDnf:
		switch action {
		case "install":
			args = []string{"dnf", "install", "-y", pkg}
		case "remove":
			args = []string{"dnf", "remove", "-y", pkg}
		case "upgrade":
			args = []string{"dnf", "upgrade", "-y", pkg}
		case "upgrade-all":
			args = []string{"dnf", "upgrade", "-y"}
		}
	case pkgPacman:
		switch action {
		case "install":
			args = []string{"pacman", "-S", "--noconfirm", pkg}
		case "remove":
			args = []string{"pacman", "-R", "--noconfirm", pkg}
		case "upgrade":
			args = []string{"pacman", "-S", "--noconfirm", pkg}
		case "upgrade-all":
			args = []string{"pacman", "-Syu", "--noconfirm"}
		}
	}

	var out string
	var err error
	if isSudoEnabled(r) {
		_, password, _ := getSessionPassword(r)
		out, err = sudoExecCommand(password, args[0], args[1:]...)
	} else {
		out, err = execCommand(args[0], args[1:]...)
	}

	label := pkg
	if action == "upgrade-all" {
		label = "all packages"
	}
	msg := action + " " + label + ": success"
	if err != nil {
		msg = action + " " + label + ": " + strings.TrimSpace(out)
		if msg == action+" "+label+": " {
			msg = action + " " + label + ": failed (try enabling sudo)"
		}
	}

	if err == nil {
		auditFromRequest(r, "Package "+action, label, "package")
	}

	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": err == nil,
			"message": msg,
			"output":  out,
		})
		return
	}

	tab := "updates"
	if action == "install" {
		tab = "search"
	} else if action == "remove" {
		tab = "installed"
	}
	http.Redirect(w, r, "/packages?tab="+tab+"&msg="+msg, http.StatusSeeOther)
}

// parsePacmanField extracts a field value from pacman -Qi output
func parsePacmanField(info, field string) string {
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, field) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				if len(val) > 100 {
					val = val[:100] + "..."
				}
				return val
			}
		}
	}
	return ""
}
