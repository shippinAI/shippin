package main

import (
	"fmt"
	"os"
	"strings"
)

type CoverageItem struct {
	ID          string `json:"id"`
	Group       string `json:"group"`
	Name        string `json:"name"`
	Why         string `json:"why"`
	Path        string `json:"path"`
	Required    bool   `json:"required"`
	Present     bool   `json:"present"`
	InList      bool   `json:"inList"`
	Enabled     bool   `json:"enabled"`
	LastStatus  string `json:"lastStatus"`
	LastTime    string `json:"lastTime"`
	StatusLabel string `json:"statusLabel"`
	ShowGroup   bool   `json:"showGroup,omitempty"`
}

type CoverageSummary struct {
	Total         int    `json:"total"`
	Present       int    `json:"present"`
	Listed        int    `json:"listed"`
	Required      int    `json:"required"`
	RequiredReady int    `json:"requiredReady"`
	RequiredGaps  int    `json:"requiredGaps"`
	Headline      string `json:"headline"`
}

func smeCoverageCatalog() []CoverageItem {
	return []CoverageItem{
		{ID: "warden-config", Group: "This panel", Name: "Panel settings", Why: "Env, deployments, templates. Without this you cannot restore Warden.", Path: "/etc/warden", Required: true},
		{ID: "warden-state", Group: "This panel", Name: "Backup records and saved keys", Why: "Job list, history, and destination keys stored on this host.", Path: "/root/.config/Warden", Required: true},
		{ID: "warden-src", Group: "This panel", Name: "Panel source", Why: "The Go tree that builds this binary. Needed to rebuild after a move.", Path: "/var/lib/shippin/warden-src/warden", Required: true},
		{ID: "warden-bin", Group: "This panel", Name: "Running panel binary", Why: "The live /usr/local/bin/warden so you can start the panel without a rebuild.", Path: "/usr/local/bin/warden", Required: true},
		{ID: "warden-unit", Group: "This panel", Name: "How the panel starts", Why: "systemd unit that launches Warden after a reboot.", Path: "/etc/systemd/system/warden.service", Required: true},
		{ID: "infisical-login", Group: "Secrets until Vaultix", Name: "Infisical login files", Why: "Cloud Infisical session used to pull secrets. Keep until Vaultix is live.", Path: "/root/.infisical", Required: true},
		{ID: "infisical-keyring", Group: "Secrets until Vaultix", Name: "Infisical keyring", Why: "Local keyring the Infisical CLI unlocks. Without this, export fails from systemd.", Path: "/root/infisical-keyring", Required: true},
		{ID: "root-ssh", Group: "Secrets until Vaultix", Name: "Root SSH keys", Why: "Host login keys. Lose these and you may not get back onto this LXC.", Path: "/root/.ssh", Required: true},
		{ID: estateTargetStorage, Group: "Shared disk and people", Name: "Shared storage", Why: "Site backup share. Set the SMB host in the site config. This panel streams it; it does not assume a disk size or a guest id.", Path: "/mnt/warden-storage", Required: true},
		{ID: "dns-data", Group: "Shared disk and people", Name: "DNS data", Why: "Optional DNS data expected under the shared mount after the share is visible.", Path: "/mnt/warden-storage/dns", Required: false},
		{ID: estateTargetHome, Group: "Shared disk and people", Name: "Operator home", Why: "Operator files for the Ryzen rebuild. Off until the path exists.", Path: "/home/wardenop", Required: false},
		{ID: "home-hades", Group: "Shared disk and people", Name: "Account home: hades", Why: "Existing operator account on this LXC.", Path: "/var/lib/shippin/operator", Required: false},
		{ID: "home-xorbit", Group: "Shared disk and people", Name: "Account home: xorbit", Why: "Existing operator account on this LXC.", Path: "/home/xorbit", Required: false},
		{ID: "node-dean", Group: "Clyffy", Name: "Clyffy dean", Why: "Head agent binary and bundled channels.", Path: "/opt/node-dean", Required: false},
		{ID: "node-dean-state", Group: "Clyffy", Name: "Clyffy dean state", Why: "Skills, persona, and policies on disk.", Path: "/var/lib/node-dean", Required: false},
		{ID: "node-dean-unit", Group: "Clyffy", Name: "Clyffy dean service", Why: "How systemd starts dean.", Path: "/etc/systemd/system/node-dean.service", Required: false},
		{ID: "node-cortex", Group: "Clyffy", Name: "Clyffy cortex", Why: "Reasoning, Discord, web UI code.", Path: "/opt/node-cortex", Required: false},
		{ID: "node-env", Group: "Clyffy", Name: "Clyffy env files", Why: "Runtime env for dean/cortex. Secrets live here today.", Path: "/etc/node-cortex", Required: false},
		{ID: "node-cortex-discord", Group: "Clyffy", Name: "Clyffy cortex Discord unit", Why: "systemd unit for the Discord process.", Path: "/etc/systemd/system/node-cortex-discord.service", Required: false},
		{ID: "node-cortex-reasoning", Group: "Clyffy", Name: "Clyffy cortex reasoning unit", Why: "systemd unit for the reasoning process.", Path: "/etc/systemd/system/node-cortex-reasoning.service", Required: false},
		{ID: "node-cortex-web", Group: "Clyffy", Name: "Clyffy cortex web unit", Why: "systemd unit for the web UI process.", Path: "/etc/systemd/system/node-cortex-web.service", Required: false},
		{ID: "warden-src-old", Group: "Other on this machine", Name: "Older panel source", Why: "Previous tree at /opt/warden-src. Keep until you are sure 0.31 is the only copy.", Path: "/opt/warden-src", Required: false},
		{ID: "proxmox-mcp", Group: "Other on this machine", Name: "Proxmox MCP", Why: "MCP connector used by this host.", Path: "/opt/proxmox-mcp", Required: false},
		{ID: "foundation-watcher", Group: "Other on this machine", Name: "Foundation watcher", Why: "Milestone watcher tree on this LXC.", Path: "/opt/foundation-milestone-watcher", Required: false},
	}
}

func coverageStatusLabel(status, when string) string {
	switch status {
	case "success":
		if when != "" {
			return "Copied OK " + when
		}
		return "Copied OK"
	case "failed":
		if when != "" {
			return "Copy failed " + when
		}
		return "Copy failed"
	case "running":
		return "Copying now"
	default:
		return "Never copied"
	}
}

func buildCoverage() []CoverageItem {
	items := smeCoverageCatalog()
	backupMu.RLock()
	targets := append([]EstateTarget(nil), backupStore.Estate.Targets...)
	history := append([]BackupRun(nil), backupStore.History...)
	backupMu.RUnlock()

	bySrc := map[string]EstateTarget{}
	byID := map[string]EstateTarget{}
	for _, t := range targets {
		bySrc[t.Source] = t
		byID[t.ID] = t
	}
	lastBySrc := map[string]BackupRun{}
	lastByID := map[string]BackupRun{}
	for _, h := range history {
		if h.Source != "" {
			if _, ok := lastBySrc[h.Source]; !ok {
				lastBySrc[h.Source] = h
			}
		}
		if h.JobID == estateJobIDFull && h.JobName != "" {
			if _, ok := lastByID[h.JobName]; !ok {
				lastByID[h.JobName] = h
			}
		}
	}

	prevGroup := ""
	for i := range items {
		st, err := os.Stat(items[i].Path)
		items[i].Present = err == nil && (st.IsDir() || st.Mode().IsRegular())
		if items[i].ID == estateTargetStorage && !items[i].Present {
			if _, _, _, _, perr := probeSMBShare(loadSMBShare()); perr == nil {
				items[i].Present = true
			}
		}
		if t, ok := byID[items[i].ID]; ok {
			items[i].InList = true
			items[i].Enabled = t.Enabled
		} else if t, ok := bySrc[items[i].Path]; ok {
			items[i].InList = true
			items[i].Enabled = t.Enabled
		}
		h, ok := lastBySrc[items[i].Path]
		if !ok {
			h, ok = lastByID[items[i].Name]
		}
		if ok {
			items[i].LastStatus = h.Status
			if h.EndTime != "" {
				items[i].LastTime = h.EndTime
			} else {
				items[i].LastTime = h.StartTime
			}
		} else {
			items[i].LastStatus = "never"
		}
		items[i].StatusLabel = coverageStatusLabel(items[i].LastStatus, items[i].LastTime)
		if items[i].Group != prevGroup {
			items[i].ShowGroup = true
			prevGroup = items[i].Group
		}
	}
	return items
}

func coverageCounts(items []CoverageItem) (present, listed, requiredMissing int) {
	for _, it := range items {
		if it.Present {
			present++
		}
		if it.InList && it.Enabled {
			listed++
		}
		if it.Required && (!it.Present || !it.InList || !it.Enabled) {
			requiredMissing++
		}
	}
	return
}

func summarizeCoverage(items []CoverageItem) CoverageSummary {
	s := CoverageSummary{Total: len(items)}
	missingDisk := 0
	notListed := 0
	for _, it := range items {
		if it.Present {
			s.Present++
		}
		if it.InList && it.Enabled {
			s.Listed++
		}
		if !it.Required {
			continue
		}
		s.Required++
		ready := it.Present && it.InList && it.Enabled
		if ready {
			s.RequiredReady++
			continue
		}
		s.RequiredGaps++
		if !it.Present {
			missingDisk++
		}
		if !it.InList || !it.Enabled {
			notListed++
		}
	}
	switch {
	case s.Required == 0:
		s.Headline = "No required items in the operator list."
	case s.RequiredGaps == 0:
		s.Headline = fmt.Sprintf("Every required item (%d) is on this machine and in the copy list.", s.Required)
	default:
		parts := []string{
			fmt.Sprintf("%d of %d required items are on this machine and enabled in the copy list.", s.RequiredReady, s.Required),
		}
		if missingDisk > 0 {
			parts = append(parts, fmt.Sprintf("%d required path(s) are not on this machine yet.", missingDisk))
		}
		if notListed > 0 {
			parts = append(parts, fmt.Sprintf("%d required item(s) still need Add or Enable.", notListed))
		}
		s.Headline = strings.Join(parts, " ")
	}
	return s
}

func upsertCoverageTargetLocked(item CoverageItem) {
	for i := range backupStore.Estate.Targets {
		if backupStore.Estate.Targets[i].ID == item.ID || backupStore.Estate.Targets[i].Source == item.Path {
			backupStore.Estate.Targets[i].Enabled = true
			backupStore.Estate.Targets[i].Source = item.Path
			backupStore.Estate.Targets[i].Name = item.Name
			if backupStore.Estate.Targets[i].Hint == "" {
				backupStore.Estate.Targets[i].Hint = item.Why
			}
			return
		}
	}
	backupStore.Estate.Targets = append(backupStore.Estate.Targets, EstateTarget{
		ID:      item.ID,
		Name:    item.Name,
		Source:  item.Path,
		Enabled: true,
		Hint:    item.Why,
	})
}
