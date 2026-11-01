package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// BlockDevice represents a partition/disk from lsblk
type BlockDevice struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Size       string `json:"size"`
	SizeBytes  uint64 `json:"sizeBytes"`
	Type       string `json:"type"`   // disk, part, lvm, crypt, rom
	FSType     string `json:"fstype"` // ext4, xfs, swap, etc
	MountPoint string `json:"mountpoint"`
	Label      string `json:"label"`
	UUID       string `json:"uuid"`
	Model      string `json:"model"`
	Serial     string `json:"serial"`
	RO         bool   `json:"ro"`
	RM         bool   `json:"rm"` // removable
	Children   []BlockDevice `json:"children,omitempty"`
}

// SMARTInfo holds SMART health data for a disk
type SMARTInfo struct {
	Device      string          `json:"device"`
	Model       string          `json:"model"`
	Serial      string          `json:"serial"`
	Firmware    string          `json:"firmware"`
	Healthy     bool            `json:"healthy"`
	Status      string          `json:"status"` // PASSED, FAILED, UNKNOWN
	Temperature string          `json:"temperature"`
	PowerOnHrs  string          `json:"powerOnHrs"`
	RawOutput   string          `json:"rawOutput"`
	Attributes  []SMARTAttribute `json:"attributes,omitempty"`
}

// SMARTAttribute is a single SMART attribute row
type SMARTAttribute struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Value     string `json:"value"`
	Worst     string `json:"worst"`
	Threshold string `json:"threshold"`
	RawValue  string `json:"rawValue"`
	Status    string `json:"status"` // ok, warn, fail
}

// DirUsage holds du output for a directory
type DirUsage struct {
	Path string `json:"path"`
	Size string `json:"size"`
	Bytes uint64 `json:"bytes"`
}

type DisksPageData struct {
	HeaderData
	MountPoints    []DiskInfo
	BlockDevices   []BlockDevice
	SMARTAvailable bool
}

func getBlockDevices() []BlockDevice {
	out, err := execCommand("lsblk", "-Jb", "-o", "NAME,PATH,SIZE,TYPE,FSTYPE,MOUNTPOINT,LABEL,UUID,MODEL,SERIAL,RO,RM")
	if err != nil {
		return nil
	}

	var result struct {
		Blockdevices []struct {
			Name       string `json:"name"`
			Path       string `json:"path"`
			Size       interface{} `json:"size"` // can be string or number
			Type       string `json:"type"`
			Fstype     *string `json:"fstype"`
			Mountpoint *string `json:"mountpoint"`
			Label      *string `json:"label"`
			UUID       *string `json:"uuid"`
			Model      *string `json:"model"`
			Serial     *string `json:"serial"`
			RO         interface{} `json:"ro"`
			RM         interface{} `json:"rm"`
			Children   []struct {
				Name       string `json:"name"`
				Path       string `json:"path"`
				Size       interface{} `json:"size"`
				Type       string `json:"type"`
				Fstype     *string `json:"fstype"`
				Mountpoint *string `json:"mountpoint"`
				Label      *string `json:"label"`
				UUID       *string `json:"uuid"`
				Model      *string `json:"model"`
				Serial     *string `json:"serial"`
				RO         interface{} `json:"ro"`
				RM         interface{} `json:"rm"`
			} `json:"children"`
		} `json:"blockdevices"`
	}

	if err := json.Unmarshal([]byte(out), &result); err != nil {
		log.Printf("lsblk parse error: %v", err)
		return nil
	}

	var devices []BlockDevice
	for _, d := range result.Blockdevices {
		dev := BlockDevice{
			Name:       d.Name,
			Path:       d.Path,
			Size:       formatBytes(parseSize(d.Size)),
			SizeBytes:  parseSize(d.Size),
			Type:       d.Type,
			FSType:     derefStr(d.Fstype),
			MountPoint: derefStr(d.Mountpoint),
			Label:      derefStr(d.Label),
			UUID:       derefStr(d.UUID),
			Model:      strings.TrimSpace(derefStr(d.Model)),
			Serial:     strings.TrimSpace(derefStr(d.Serial)),
			RO:         parseBoolish(d.RO),
			RM:         parseBoolish(d.RM),
		}

		for _, c := range d.Children {
			child := BlockDevice{
				Name:       c.Name,
				Path:       c.Path,
				Size:       formatBytes(parseSize(c.Size)),
				SizeBytes:  parseSize(c.Size),
				Type:       c.Type,
				FSType:     derefStr(c.Fstype),
				MountPoint: derefStr(c.Mountpoint),
				Label:      derefStr(c.Label),
				UUID:       derefStr(c.UUID),
				Model:      strings.TrimSpace(derefStr(c.Model)),
				Serial:     strings.TrimSpace(derefStr(c.Serial)),
				RO:         parseBoolish(c.RO),
				RM:         parseBoolish(c.RM),
			}
			dev.Children = append(dev.Children, child)
		}

		devices = append(devices, dev)
	}

	return devices
}

func getSMARTInfo(device string) *SMARTInfo {
	// Sanitize device path
	if !strings.HasPrefix(device, "/dev/") {
		return nil
	}
	if strings.ContainsAny(device, " ;|&$`\\") {
		return nil
	}

	out, err := execCommand("smartctl", "-a", device)
	// smartctl returns non-zero for some valid outputs, so check output too
	if err != nil && out == "" {
		return nil
	}

	info := &SMARTInfo{
		Device:    device,
		Status:    "UNKNOWN",
		RawOutput: out,
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Device Model:") || strings.HasPrefix(line, "Model Number:") {
			info.Model = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		} else if strings.HasPrefix(line, "Serial Number:") {
			info.Serial = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		} else if strings.HasPrefix(line, "Firmware Version:") {
			info.Firmware = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		} else if strings.HasPrefix(line, "SMART overall-health") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				status := strings.TrimSpace(parts[1])
				info.Status = status
				info.Healthy = status == "PASSED"
			}
		} else if strings.HasPrefix(line, "Temperature:") || strings.Contains(line, "Temperature_Celsius") {
			// Try to extract temperature
			fields := strings.Fields(line)
			if len(fields) > 0 {
				for _, f := range fields {
					if n, err := strconv.Atoi(f); err == nil && n > 0 && n < 150 {
						info.Temperature = strconv.Itoa(n) + "°C"
						break
					}
				}
			}
		} else if strings.Contains(line, "Power_On_Hours") {
			fields := strings.Fields(line)
			if len(fields) >= 10 {
				info.PowerOnHrs = fields[9]
			}
		}
	}

	// Parse SMART attributes table
	inTable := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "ID#") {
			inTable = true
			continue
		}
		if inTable {
			if line == "" || !strings.Contains(line, " ") {
				inTable = false
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 10 {
				attr := SMARTAttribute{
					ID:        fields[0],
					Name:      fields[1],
					Value:     fields[3],
					Worst:     fields[4],
					Threshold: fields[5],
					RawValue:  fields[9],
					Status:    "ok",
				}
				// Check if value is below threshold
				val, _ := strconv.Atoi(fields[3])
				thresh, _ := strconv.Atoi(fields[5])
				if thresh > 0 && val <= thresh {
					attr.Status = "fail"
				} else if thresh > 0 && val <= thresh+10 {
					attr.Status = "warn"
				}
				info.Attributes = append(info.Attributes, attr)
			}
		}
	}

	return info
}

func getDirUsage(path string) []DirUsage {
	// Sanitize path
	if strings.ContainsAny(path, ";|&$`\\") {
		return nil
	}

	out, err := execCommand("du", "-b", "--max-depth=1", "-t", "1048576", path)
	if err != nil {
		// Try without -b (some systems don't support it)
		out, err = execCommand("du", "--max-depth=1", "-k", path)
		if err != nil {
			return nil
		}
	}

	var entries []DirUsage
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		bytes, _ := strconv.ParseUint(fields[0], 10, 64)
		p := strings.Join(fields[1:], " ")

		// Skip the root path itself (it's the total)
		if p == path {
			continue
		}

		entries = append(entries, DirUsage{
			Path:  p,
			Size:  formatBytes(bytes),
			Bytes: bytes,
		})
	}

	// Sort by size descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Bytes > entries[j].Bytes
	})

	return entries
}

func isSMARTAvailable() bool {
	_, err := execCommand("which", "smartctl")
	return err == nil
}

func handleDisks(w http.ResponseWriter, r *http.Request) {
	hd := buildHeaderData(r)
	hd.PageTitle = "Disks"

	data := DisksPageData{
		HeaderData:     hd,
		MountPoints:    getDiskInfo(),
		BlockDevices:   getBlockDevices(),
		SMARTAvailable: isSMARTAvailable(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "disks.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIDisks(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")

	switch action {
	case "smart":
		device := r.URL.Query().Get("device")
		if device == "" {
			jsonError(w, "Device required")
			return
		}

		if !isSudoEnabled(r) {
			jsonError(w, "SMART requires elevated privileges. Enable the sudo toggle first.")
			return
		}

		_, password, _ := getSessionPassword(r)
		out, err := sudoExecCommand(password, "smartctl", "-a", device)
		if err != nil && out == "" {
			jsonError(w, "SMART query failed: "+err.Error())
			return
		}
		info := &SMARTInfo{Device: device, Status: "UNKNOWN", RawOutput: out}
		parseSMARTOutput(info, out)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"smart":   info,
		})

	case "usage":
		path := r.URL.Query().Get("path")
		if path == "" {
			path = "/"
		}
		entries := getDirUsage(path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"entries": entries,
			"path":    path,
		})

	default:
		// Return all disk info
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mountPoints":  getDiskInfo(),
			"blockDevices": getBlockDevices(),
			"smart":        isSMARTAvailable(),
		})
	}
}

func parseSMARTOutput(info *SMARTInfo, out string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Device Model:") || strings.HasPrefix(line, "Model Number:") {
			info.Model = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		} else if strings.HasPrefix(line, "Serial Number:") {
			info.Serial = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		} else if strings.HasPrefix(line, "Firmware Version:") {
			info.Firmware = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		} else if strings.HasPrefix(line, "SMART overall-health") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				status := strings.TrimSpace(parts[1])
				info.Status = status
				info.Healthy = status == "PASSED"
			}
		} else if strings.Contains(line, "Temperature_Celsius") || strings.HasPrefix(line, "Temperature:") {
			fields := strings.Fields(line)
			for _, f := range fields {
				if n, err := strconv.Atoi(f); err == nil && n > 0 && n < 150 {
					info.Temperature = strconv.Itoa(n) + "°C"
					break
				}
			}
		} else if strings.Contains(line, "Power_On_Hours") {
			fields := strings.Fields(line)
			if len(fields) >= 10 {
				info.PowerOnHrs = fields[9]
			}
		}
	}

	// Parse attributes table
	inTable := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "ID#") {
			inTable = true
			continue
		}
		if inTable {
			if line == "" || !strings.Contains(line, " ") {
				inTable = false
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 10 {
				attr := SMARTAttribute{
					ID: fields[0], Name: fields[1],
					Value: fields[3], Worst: fields[4], Threshold: fields[5],
					RawValue: fields[9], Status: "ok",
				}
				val, _ := strconv.Atoi(fields[3])
				thresh, _ := strconv.Atoi(fields[5])
				if thresh > 0 && val <= thresh {
					attr.Status = "fail"
				} else if thresh > 0 && val <= thresh+10 {
					attr.Status = "warn"
				}
				info.Attributes = append(info.Attributes, attr)
			}
		}
	}
}

// Helper functions

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func parseSize(v interface{}) uint64 {
	switch val := v.(type) {
	case float64:
		return uint64(val)
	case string:
		n, _ := strconv.ParseUint(val, 10, 64)
		return n
	}
	return 0
}

func parseBoolish(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case string:
		return val == "1" || val == "true"
	}
	return false
}

// formatBytes is defined in main.go
