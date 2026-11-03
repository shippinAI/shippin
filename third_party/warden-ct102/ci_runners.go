package main

// CI runner fleet registry + composed health.
// Spec: docs/specs/runner-fleet-health-and-stats.md §2 (health layer, P1).
//
// Three signals per runner, probed concurrently on request:
//   GitHub    — runner online/busy per served repo (needs WARDEN_GITHUB_TOKEN;
//               absent token degrades the signal to unknown, never to red)
//   Substrate — guest running + (VMs) QEMU guest-agent ping via Proxmox API
//   Host      — CPU/mem pressure from the hypervisor's own accounting
// Health history is not stored; the stats layer (P2) implies it.

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type CIRunner struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Kind  string   `json:"kind"` // qemu | lxc
	VMID  int      `json:"vmid"`
	Host  string   `json:"host"`
	Repos []string `json:"repos"`
}

type CIRunnerSignal struct {
	State  HealthState `json:"state"`
	Detail string      `json:"detail,omitempty"`
}

type CIRunnerHealth struct {
	CIRunner
	GitHub    CIRunnerSignal `json:"github"`
	Substrate CIRunnerSignal `json:"substrate"`
	HostLoad  CIRunnerSignal `json:"host_load"`
	Overall   HealthState    `json:"overall"`
	CheckedAt time.Time      `json:"checked_at"`
}

func defaultCIRunners() []CIRunner {
	repos := []string{"example/repo"}
	return []CIRunner{
		{ID: "runner-a", Name: "runner-a", Kind: "qemu", VMID: 100, Host: "192.0.2.40", Repos: repos},
		{ID: "runner-b", Name: "runner-b", Kind: "lxc", VMID: 101, Host: "192.0.2.41", Repos: repos},
	}
}

func loadCIRunners() []CIRunner {
	path := filepath.Join(filepath.Dir(platformPath), "ci_runners.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultCIRunners()
	}
	var store struct {
		Runners []CIRunner `json:"runners"`
	}
	if err := json.Unmarshal(data, &store); err != nil || len(store.Runners) == 0 {
		log.Printf("Failed to parse ci_runners.json, using defaults: %v", err)
		return defaultCIRunners()
	}
	return store.Runners
}

// --- GitHub signal (cached fleet-wide) -----------------------------------

var (
	ghRunnersMu      sync.Mutex
	ghRunnersCache   map[string]string // "repo|runner-name" -> "online"|"online-busy"|"offline"
	ghRunnersFetched time.Time
)

func githubRunnerStates(repos []string) map[string]string {
	token := strings.TrimSpace(os.Getenv("WARDEN_GITHUB_TOKEN"))
	if token == "" {
		return nil
	}
	ghRunnersMu.Lock()
	defer ghRunnersMu.Unlock()
	if ghRunnersCache != nil && time.Since(ghRunnersFetched) < 60*time.Second {
		return ghRunnersCache
	}
	out := map[string]string{}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, repo := range repos {
		req, err := http.NewRequest("GET", "https://api.github.com/repos/"+repo+"/actions/runners?per_page=100", nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("GitHub runner probe failed for %q: %v", repo, err)
			continue
		}
		var body struct {
			Runners []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
				Busy   bool   `json:"busy"`
			} `json:"runners"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			log.Printf("GitHub runner probe for %q: HTTP %d, %v", repo, resp.StatusCode, err)
			continue
		}
		for _, ru := range body.Runners {
			state := ru.Status
			if ru.Busy {
				state = state + "-busy"
			}
			out[repo+"|"+ru.Name] = state
		}
	}
	ghRunnersCache = out
	ghRunnersFetched = time.Now()
	return out
}

// --- probes ---------------------------------------------------------------

func probeGitHub(r CIRunner, states map[string]string) CIRunnerSignal {
	if states == nil {
		return CIRunnerSignal{State: HealthUnknown, Detail: "WARDEN_GITHUB_TOKEN not set"}
	}
	details := make([]string, 0, len(r.Repos))
	worst := HealthOK
	for _, repo := range r.Repos {
		st, ok := states[repo+"|"+r.Name]
		if !ok {
			st = "unregistered"
		}
		details = append(details, fmt.Sprintf("%s: %s", repo, st))
		if !strings.HasPrefix(st, "online") {
			worst = HealthCritical
		}
	}
	return CIRunnerSignal{State: worst, Detail: strings.Join(details, "; ")}
}

func probeSubstrate(r CIRunner) (CIRunnerSignal, CIRunnerSignal) {
	cfg, err := getProxmoxConfig()
	if err != nil {
		return CIRunnerSignal{State: HealthUnknown, Detail: "proxmox not configured"},
			CIRunnerSignal{State: HealthUnknown}
	}
	path := fmt.Sprintf("/nodes/%s/%s/%d/status/current", cfg.Node, r.Kind, r.VMID)
	data, code, err := proxmoxAPI("GET", path, nil)
	if err != nil || code != 200 {
		return CIRunnerSignal{State: HealthUnknown, Detail: fmt.Sprintf("status probe HTTP %d: %v", code, err)},
			CIRunnerSignal{State: HealthUnknown}
	}
	var body struct {
		Data struct {
			Status string  `json:"status"`
			CPU    float64 `json:"cpu"`
			Mem    int64   `json:"mem"`
			MaxMem int64   `json:"maxmem"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return CIRunnerSignal{State: HealthUnknown, Detail: err.Error()},
			CIRunnerSignal{State: HealthUnknown}
	}
	sub := CIRunnerSignal{State: HealthOK, Detail: body.Data.Status}
	if body.Data.Status != "running" {
		sub.State = HealthCritical
		return sub, CIRunnerSignal{State: HealthUnknown}
	}
	if r.Kind == "qemu" {
		if _, code, err := proxmoxAPI("POST", fmt.Sprintf("/nodes/%s/qemu/%d/agent/ping", cfg.Node, r.VMID), url.Values{}); err != nil || code != 200 {
			sub.State = HealthWarn
			sub.Detail = fmt.Sprintf("running, guest agent not answering (HTTP %d)", code)
		} else {
			sub.Detail = "running, guest agent alive"
		}
	}
	cpuPct := body.Data.CPU * 100
	memPct := 0.0
	if body.Data.MaxMem > 0 {
		memPct = float64(body.Data.Mem) / float64(body.Data.MaxMem) * 100
	}
	load := CIRunnerSignal{
		State:  HealthOK,
		Detail: fmt.Sprintf("cpu %.0f%%, mem %.0f%%", cpuPct, memPct),
	}
	if cpuPct >= 85 || memPct >= 85 {
		load.State = HealthWarn
	}
	return sub, load
}

// composeRunnerHealth is the pill rule from the spec: substrate dead or
// GitHub-offline is red; any single degraded signal is amber; probed and
// fine is green; nothing probeable is unknown.
func composeRunnerHealth(github, substrate, hostLoad HealthState) HealthState {
	if substrate == HealthCritical || github == HealthCritical {
		return HealthCritical
	}
	if github == HealthWarn || substrate == HealthWarn || hostLoad == HealthWarn {
		return HealthWarn
	}
	if github == HealthUnknown && substrate == HealthUnknown && hostLoad == HealthUnknown {
		return HealthUnknown
	}
	return HealthOK
}

func collectCIRunnerHealth() []CIRunnerHealth {
	runners := loadCIRunners()
	repoSet := map[string]bool{}
	for _, r := range runners {
		for _, repo := range r.Repos {
			repoSet[repo] = true
		}
	}
	repos := make([]string, 0, len(repoSet))
	for repo := range repoSet {
		repos = append(repos, repo)
	}
	ghStates := githubRunnerStates(repos)

	out := make([]CIRunnerHealth, len(runners))
	var wg sync.WaitGroup
	for i, r := range runners {
		wg.Add(1)
		go func(i int, r CIRunner) {
			defer wg.Done()
			gh := probeGitHub(r, ghStates)
			sub, load := probeSubstrate(r)
			out[i] = CIRunnerHealth{
				CIRunner:  r,
				GitHub:    gh,
				Substrate: sub,
				HostLoad:  load,
				Overall:   composeRunnerHealth(gh.State, sub.State, load.State),
				CheckedAt: time.Now().UTC(),
			}
		}(i, r)
	}
	wg.Wait()
	return out
}
