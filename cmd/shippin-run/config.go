package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"shippin.cloud/console/internal/runfleet"
)

var appSlug = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

func installURL(slug string) (string, error) {
	if !appSlug.MatchString(slug) {
		return "", fmt.Errorf("SHIPPIN_RUN_GITHUB_APP_SLUG must be a GitHub App slug")
	}
	return "https://github.com/apps/" + slug + "/installations/new", nil
}

type serverConfig struct {
	Listen        string
	Token         string
	WebhookSecret string
	StateDir      string
}

type fileConfig struct {
	Listen                  string     `json:"listen"`
	StateDir                string     `json:"state_dir"`
	FreeMinutes             int        `json:"free_minutes"`
	FreeSize                string     `json:"free_size"`
	DefaultLogRetentionDays int        `json:"default_log_retention_days"`
	Provisioner             string     `json:"provisioner"`
	Sizes                   []sizeFile `json:"sizes"`
}

type sizeFile struct {
	ID           string `json:"id"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	VCPU         int    `json:"vcpu"`
	MemoryMiB    int    `json:"memory_mib"`
	USDPerMinute string `json:"usd_per_minute"`
	Note         string `json:"note"`
}

func openFromEnv() (*runfleet.Service, serverConfig, error) {
	var file fileConfig
	if path := strings.TrimSpace(os.Getenv("SHIPPIN_RUN_CONFIG")); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, serverConfig{}, err
		}
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&file); err != nil {
			return nil, serverConfig{}, fmt.Errorf("config: %w", err)
		}
	}
	cfg := serverConfig{
		Listen:        first(os.Getenv("SHIPPIN_RUN_LISTEN"), file.Listen, ":8080"),
		Token:         os.Getenv("SHIPPIN_RUN_API_TOKEN"),
		WebhookSecret: os.Getenv("SHIPPIN_RUN_GITHUB_WEBHOOK_SECRET"),
		StateDir:      first(os.Getenv("SHIPPIN_RUN_STATE_DIR"), file.StateDir),
	}
	if cfg.StateDir == "" {
		return nil, cfg, fmt.Errorf("SHIPPIN_RUN_STATE_DIR is required")
	}
	opt := runfleet.Options{
		FreeMinutes:      file.FreeMinutes,
		FreeSize:         file.FreeSize,
		DefaultRetention: file.DefaultLogRetentionDays,
		Provisioner:      provisionerFrom(first(os.Getenv("SHIPPIN_RUN_PROVISIONER"), file.Provisioner)),
	}
	if v := os.Getenv("SHIPPIN_RUN_FREE_MINUTES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, cfg, fmt.Errorf("SHIPPIN_RUN_FREE_MINUTES must be a non-negative integer")
		}
		if n == 0 {
			opt.FreeMinutes = -1
		} else {
			opt.FreeMinutes = n
		}
	}
	if len(file.Sizes) > 0 {
		sizes := make([]runfleet.Size, 0, len(file.Sizes))
		for _, row := range file.Sizes {
			sz, err := runfleet.NewSize(row.ID, row.OS, row.Arch, row.VCPU, row.MemoryMiB, row.USDPerMinute, row.Note)
			if err != nil {
				return nil, cfg, err
			}
			sizes = append(sizes, sz)
		}
		opt.Sizes = sizes
	}
	if gh, err := githubFromEnv(); err != nil {
		return nil, cfg, err
	} else {
		opt.GitHub = gh
	}
	svc, err := runfleet.Open(cfg.StateDir, opt)
	return svc, cfg, err
}

func githubFromEnv() (runfleet.GitHub, error) {
	appID := strings.TrimSpace(os.Getenv("SHIPPIN_RUN_GITHUB_APP_ID"))
	keyPath := strings.TrimSpace(os.Getenv("SHIPPIN_RUN_GITHUB_PRIVATE_KEY_FILE"))
	inst := strings.TrimSpace(os.Getenv("SHIPPIN_RUN_GITHUB_INSTALLATION_ID"))
	token := strings.TrimSpace(os.Getenv("SHIPPIN_RUN_GITHUB_TOKEN"))
	if appID == "" && keyPath == "" && inst == "" && token == "" {
		return nil, nil
	}
	gh := &runfleet.HTTPGitHub{Token: token, AppID: appID}
	if inst != "" {
		n, err := strconv.ParseInt(inst, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("SHIPPIN_RUN_GITHUB_INSTALLATION_ID must be a positive integer")
		}
		gh.InstallationID = n
	}
	if keyPath != "" {
		key, err := readPrivateKey(keyPath)
		if err != nil {
			return nil, err
		}
		gh.PrivateKey = key
	}
	if !gh.Configured() {
		return nil, fmt.Errorf("GitHub settings are incomplete: set SHIPPIN_RUN_GITHUB_TOKEN, or the app id, private key file, and installation id")
	}
	return gh, nil
}

func readPrivateKey(path string) (*rsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, fmt.Errorf("private key file is not PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	return key, nil
}

func provisionerFrom(name string) runfleet.Provisioner {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "proxmox":
		return runfleet.Proxmox{
			BaseURL: os.Getenv("SHIPPIN_RUN_PROXMOX_API_URL"),
			Token:   os.Getenv("SHIPPIN_RUN_PROXMOX_TOKEN"),
			Node:    os.Getenv("SHIPPIN_RUN_PROXMOX_NODE"),
		}
	case "incus":
		return runfleet.Incus{}
	default:
		return runfleet.Unconfigured{}
	}
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
