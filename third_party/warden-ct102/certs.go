package main

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CertInfo struct {
	Path       string   `json:"path"`
	Domain     string   `json:"domain"`
	SANs       []string `json:"sans"`
	Issuer     string   `json:"issuer"`
	NotBefore  string   `json:"notBefore"`
	NotAfter   string   `json:"notAfter"`
	DaysLeft   int      `json:"daysLeft"`
	Status     string   `json:"status"` // valid, expiring, expired
	Serial     string   `json:"serial"`
	BarWidth   int      `json:"barWidth"`
	IsLE       bool     `json:"isLE"`
	LECertName string   `json:"leCertName,omitempty"`
}

type CertbotInfo struct {
	Available    bool   `json:"available"`
	Version      string `json:"version"`
	TimerActive  bool   `json:"timerActive"`
	TimerStatus  string `json:"timerStatus"`
}

type CertsPageData struct {
	HeaderData
	Certs    []CertInfo
	Certbot  CertbotInfo
}

// Common certificate directories to scan
var certSearchPaths = []string{
	"/etc/letsencrypt/live",
	"/etc/ssl/certs",
	"/etc/ssl/private",
	"/etc/nginx/ssl",
	"/etc/apache2/ssl",
	"/etc/pki/tls/certs",
	"/etc/certs",
}

func scanCertificates() []CertInfo {
	var certs []CertInfo
	seen := make(map[string]bool)

	for _, dir := range certSearchPaths {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		// Special handling for Let's Encrypt live directory
		if strings.Contains(dir, "letsencrypt/live") {
			certs = append(certs, scanLetsEncryptLive(dir, seen)...)
			continue
		}

		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			// Only scan .pem, .crt, .cert files
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".pem" && ext != ".crt" && ext != ".cert" {
				return nil
			}
			// Skip private keys, CA bundles, and symlinks to system CA store
			base := strings.ToLower(filepath.Base(path))
			if strings.Contains(base, "key") || strings.Contains(base, "ca-certificates") || strings.Contains(base, "ca-bundle") {
				return nil
			}
			// Skip system CA certs directory (too many)
			if strings.HasPrefix(path, "/etc/ssl/certs/") && !strings.Contains(path, "local") {
				// Only scan custom certs in /etc/ssl/certs, not the hundreds of system CA certs
				if info.Size() < 2048 {
					return nil
				}
			}

			if seen[path] {
				return nil
			}

			ci := parseCertFile(path)
			if ci != nil {
				seen[path] = true
				certs = append(certs, *ci)
			}
			return nil
		})
	}

	// Sort by days left (most urgent first)
	sort.Slice(certs, func(i, j int) bool {
		return certs[i].DaysLeft < certs[j].DaysLeft
	})

	return certs
}

func scanLetsEncryptLive(dir string, seen map[string]bool) []CertInfo {
	var certs []CertInfo

	entries, err := os.ReadDir(dir)
	if err != nil {
		return certs
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		certPath := filepath.Join(dir, entry.Name(), "cert.pem")
		fullchainPath := filepath.Join(dir, entry.Name(), "fullchain.pem")

		// Prefer fullchain, fall back to cert
		path := fullchainPath
		if _, err := os.Stat(path); os.IsNotExist(err) {
			path = certPath
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}

		if seen[path] {
			continue
		}

		ci := parseCertFile(path)
		if ci != nil {
			ci.IsLE = true
			ci.LECertName = entry.Name()
			seen[path] = true
			certs = append(certs, *ci)
		}
	}

	return certs
}

func parseCertFile(path string) *CertInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}

	// Skip CA certificates
	if cert.IsCA {
		return nil
	}

	now := time.Now()
	daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)

	status := "valid"
	if daysLeft <= 0 {
		status = "expired"
	} else if daysLeft <= 30 {
		status = "expiring"
	}

	domain := cert.Subject.CommonName
	if domain == "" && len(cert.DNSNames) > 0 {
		domain = cert.DNSNames[0]
	}

	issuer := cert.Issuer.Organization
	issuerStr := "Unknown"
	if len(issuer) > 0 {
		issuerStr = issuer[0]
	} else if cert.Issuer.CommonName != "" {
		issuerStr = cert.Issuer.CommonName
	}

	barWidth := 0
	if daysLeft > 0 {
		barWidth = daysLeft * 100 / 90
		if barWidth > 100 {
			barWidth = 100
		}
	}

	// Check if it's a Let's Encrypt cert by issuer
	isLE := strings.Contains(strings.ToLower(issuerStr), "let's encrypt") ||
		strings.Contains(strings.ToLower(issuerStr), "letsencrypt") ||
		strings.Contains(strings.ToLower(cert.Issuer.CommonName), "let's encrypt")

	return &CertInfo{
		Path:      path,
		Domain:    domain,
		SANs:      cert.DNSNames,
		Issuer:    issuerStr,
		NotBefore: cert.NotBefore.Format("2006-01-02"),
		NotAfter:  cert.NotAfter.Format("2006-01-02"),
		DaysLeft:  daysLeft,
		Status:    status,
		Serial:    fmt.Sprintf("%X", cert.SerialNumber),
		BarWidth:  barWidth,
		IsLE:      isLE,
	}
}

func getCertbotInfo() CertbotInfo {
	info := CertbotInfo{}

	// Check if certbot is installed
	version, err := execCommand("certbot", "--version")
	if err != nil {
		return info
	}
	info.Available = true
	info.Version = strings.TrimSpace(version)

	// Check certbot timer
	timerOut, err := execCommand("systemctl", "is-active", "certbot.timer")
	if err == nil {
		status := strings.TrimSpace(timerOut)
		info.TimerActive = status == "active"
		info.TimerStatus = status
	} else {
		// Try snap certbot timer
		timerOut, err = execCommand("systemctl", "is-active", "snap.certbot.renew.timer")
		if err == nil {
			status := strings.TrimSpace(timerOut)
			info.TimerActive = status == "active"
			info.TimerStatus = status
		} else {
			info.TimerStatus = "not found"
		}
	}

	return info
}

func handleCerts(w http.ResponseWriter, r *http.Request) {
	hd := buildHeaderData(r)
	hd.PageTitle = "Certificates"

	data := CertsPageData{
		HeaderData: hd,
		Certs:      scanCertificates(),
		Certbot:    getCertbotInfo(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "certs.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPICerts(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")

	switch r.Method {
	case http.MethodGet:
		if action == "certbot-certs" {
			out, err := execCommand("certbot", "certificates")
			if err != nil {
				jsonError(w, "Failed to list certbot certificates: "+err.Error())
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"output":  out,
			})
			return
		}

		// Default: return scanned certs
		certs := scanCertificates()
		certbot := getCertbotInfo()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"certs":   certs,
			"certbot": certbot,
		})
		return

	case http.MethodPost:
		r.ParseForm()

		switch action {
		case "renew":
			certName := r.FormValue("certName")
			if certName == "" {
				jsonError(w, "Certificate name required")
				return
			}
			// Sanitize cert name
			if strings.ContainsAny(certName, "/\\;|&$`") {
				jsonError(w, "Invalid certificate name")
				return
			}

			sudo := isSudoEnabled(r)
			var out string
			var err error
			if sudo {
				_, password, _ := getSessionPassword(r)
				out, err = sudoExecCommand(password, "certbot", "renew", "--cert-name", certName, "--non-interactive")
			} else {
				out, err = execCommand("certbot", "renew", "--cert-name", certName, "--non-interactive")
			}
			if err != nil {
				jsonError(w, "Renewal failed: "+err.Error()+"\n"+out)
				return
			}
			auditFromRequest(r, "Certificate renewed", certName, "system")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Certificate renewed successfully",
				"output":  out,
			})

		case "renew-all":
			sudo := isSudoEnabled(r)
			var out string
			var err error
			if sudo {
				_, password, _ := getSessionPassword(r)
				out, err = sudoExecCommand(password, "certbot", "renew", "--non-interactive")
			} else {
				out, err = execCommand("certbot", "renew", "--non-interactive")
			}
			if err != nil {
				jsonError(w, "Renewal failed: "+err.Error()+"\n"+out)
				return
			}
			auditFromRequest(r, "All certificates renewed", "", "system")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "All certificates renewed",
				"output":  out,
			})

		case "request":
			domain := r.FormValue("domain")
			method := r.FormValue("method") // standalone, webroot
			webroot := r.FormValue("webroot")
			email := r.FormValue("email")

			if domain == "" {
				jsonError(w, "Domain required")
				return
			}
			// Sanitize domain
			if strings.ContainsAny(domain, " ;|&$`\\") {
				jsonError(w, "Invalid domain name")
				return
			}

			args := []string{"certonly", "--non-interactive", "--agree-tos"}
			if email != "" {
				if strings.ContainsAny(email, " ;|&$`\\") {
					jsonError(w, "Invalid email")
					return
				}
				args = append(args, "--email", email)
			} else {
				args = append(args, "--register-unsafely-without-email")
			}

			if method == "webroot" && webroot != "" {
				if strings.ContainsAny(webroot, ";|&$`\\") {
					jsonError(w, "Invalid webroot path")
					return
				}
				args = append(args, "--webroot", "-w", webroot)
			} else {
				args = append(args, "--standalone")
			}

			// Support multiple domains
			for _, d := range strings.Split(domain, ",") {
				d = strings.TrimSpace(d)
				if d != "" {
					args = append(args, "-d", d)
				}
			}

			sudo := isSudoEnabled(r)
			var out string
			var err error
			if sudo {
				_, password, _ := getSessionPassword(r)
				out, err = sudoExecCommand(password, "certbot", args...)
			} else {
				out, err = execCommand("certbot", args...)
			}
			if err != nil {
				jsonError(w, "Certificate request failed: "+err.Error()+"\n"+out)
				return
			}
			auditFromRequest(r, "Certificate requested", domain, "system")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Certificate obtained for " + domain,
				"output":  out,
			})

		default:
			jsonError(w, "Unknown action")
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func jsonError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   msg,
	})
}
