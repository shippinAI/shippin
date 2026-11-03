package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	smbDefaultHost  = ""
	smbDefaultShare = "warden-storage"
	smbDefaultMount = "/mnt/warden-storage"
	smbCredName     = "smb-warden-storage.cred"
)

type smbShare struct {
	Host     string
	Share    string
	UNC      string
	User     string
	CredFile string
	Mount    string
	HaveUser bool
	HavePass bool
}

type StorageStatus struct {
	Name         string `json:"name"`
	Mount        string `json:"mount"`
	UNC          string `json:"unc"`
	Host         string `json:"host"`
	LocalMounted bool   `json:"localMounted"`
	SMBReady     bool   `json:"smbReady"`
	CopyReady    bool   `json:"copyReady"`
	UsedBytes    uint64 `json:"usedBytes"`
	UsedHuman    string `json:"usedHuman,omitempty"`
	FreeHuman    string `json:"freeHuman,omitempty"`
	Message      string `json:"message"`
	LastStatus   string `json:"lastStatus,omitempty"`
	LastTime     string `json:"lastTime,omitempty"`
}

func smbCredPath() string {
	if p := strings.TrimSpace(os.Getenv("WARDEN_SMB_CRED_FILE")); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(backupPath), smbCredName)
}

func loadSMBShare() smbShare {
	s := smbShare{
		Host:     strings.TrimSpace(os.Getenv("WARDEN_SHARED_STORAGE_01_SMB_HOST")),
		Share:    strings.TrimSpace(os.Getenv("WARDEN_SHARED_STORAGE_01_SMB_SHARE")),
		UNC:      strings.TrimSpace(os.Getenv("WARDEN_SHARED_STORAGE_01_SMB_UNC")),
		User:     strings.TrimSpace(os.Getenv("WARDEN_SHARED_STORAGE_01_SMB_USERNAME")),
		Mount:    strings.TrimSpace(os.Getenv("WARDEN_SHARED_STORAGE_01_LINUX_MOUNT")),
		CredFile: smbCredPath(),
	}
	if s.Host == "" {
		s.Host = smbDefaultHost
	}
	if s.Share == "" {
		s.Share = smbDefaultShare
	}
	if s.Mount == "" {
		s.Mount = smbDefaultMount
	}
	if s.UNC == "" && s.Host != "" && s.Share != "" {
		s.UNC = "//" + s.Host + "/" + s.Share
	}
	pass := strings.TrimSpace(os.Getenv("WARDEN_SHARED_STORAGE_01_SMB_PASSWORD"))
	s.HaveUser = s.User != ""
	s.HavePass = pass != ""
	if data, err := os.ReadFile(s.CredFile); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.ToLower(strings.TrimSpace(k))
			v = strings.TrimSpace(v)
			if k == "username" && v != "" {
				if s.User == "" {
					s.User = v
				}
				s.HaveUser = true
			}
			if k == "password" && v != "" {
				s.HavePass = true
			}
		}
	}
	return s
}

func (s smbShare) missing() string {
	var miss []string
	if s.UNC == "" || s.UNC == "//" {
		miss = append(miss, "share address")
	}
	if !s.HaveUser {
		miss = append(miss, "username")
	}
	if !s.HavePass {
		miss = append(miss, "password")
	}
	if len(miss) == 0 {
		return ""
	}
	return "SMB login incomplete: missing " + strings.Join(miss, ", ")
}

var (
	smbProbeMu      sync.Mutex
	smbProbeAt      time.Time
	smbProbeUsed    uint64
	smbProbeTotal   uint64
	smbProbeAvail   uint64
	smbProbeListing string
	smbProbeErr     error
)

func probeSMBShare(s smbShare) (used, total, avail uint64, listing string, err error) {
	smbProbeMu.Lock()
	if time.Since(smbProbeAt) < 20*time.Second && smbProbeAt.IsZero() == false {
		used, total, avail, listing, err = smbProbeUsed, smbProbeTotal, smbProbeAvail, smbProbeListing, smbProbeErr
		smbProbeMu.Unlock()
		return
	}
	smbProbeMu.Unlock()

	used, total, avail, listing, err = probeSMBShareUncached(s)

	smbProbeMu.Lock()
	smbProbeAt = time.Now()
	smbProbeUsed, smbProbeTotal, smbProbeAvail = used, total, avail
	smbProbeListing, smbProbeErr = listing, err
	smbProbeMu.Unlock()
	return
}

func probeSMBShareUncached(s smbShare) (used, total, avail uint64, listing string, err error) {
	if msg := s.missing(); msg != "" {
		return 0, 0, 0, "", fmt.Errorf("%s", msg)
	}
	if _, err := exec.LookPath("smbclient"); err != nil {
		return 0, 0, 0, "", fmt.Errorf("smbclient is not installed on this panel")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "smbclient", s.UNC, "-A", s.CredFile, "-c", "ls")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if ctx.Err() == context.DeadlineExceeded {
		return 0, 0, 0, "", fmt.Errorf("SMB list of %s timed out", s.UNC)
	}
	if err != nil {
		low := strings.ToLower(text + " " + err.Error())
		switch {
		case strings.Contains(low, "logon failure") || strings.Contains(low, "access denied") || strings.Contains(low, "nt_status_logon"):
			return 0, 0, 0, "", fmt.Errorf("SMB login failed for %s (username/password rejected)", s.UNC)
		case strings.Contains(low, "nt_status_bad_network_name"):
			return 0, 0, 0, "", fmt.Errorf("SMB share name %q was not found on %s", s.Share, s.Host)
		case strings.Contains(low, "connection refused") || strings.Contains(low, "timed out"):
			return 0, 0, 0, "", fmt.Errorf("cannot reach SMB host %s:445", s.Host)
		default:
			return 0, 0, 0, "", fmt.Errorf("SMB list failed for %s", s.UNC)
		}
	}
	total, avail, used, _ = parseSMBBlocks(text)
	return used, total, avail, summarizeSMBListing(text), nil
}

func parseSMBBlocks(output string) (total, avail, used uint64, ok bool) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// "411725224 blocks of size 1024. 388027056 blocks available"
		if !strings.Contains(line, "blocks of size") || !strings.Contains(line, "blocks available") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		tb, e1 := strconv.ParseUint(fields[0], 10, 64)
		bs, e2 := strconv.ParseUint(strings.TrimRight(fields[4], "."), 10, 64)
		ab, e3 := strconv.ParseUint(fields[5], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil || bs == 0 {
			continue
		}
		total = tb * bs
		avail = ab * bs
		if total > avail {
			used = total - avail
		}
		return total, avail, used, true
	}
	return 0, 0, 0, false
}

func summarizeSMBListing(output string) string {
	var names []string
	sc := bufio.NewScanner(strings.NewReader(output))
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, " D ") && !strings.Contains(line, " N ") && !strings.Contains(line, " A ") {
			continue
		}
		name := strings.TrimSpace(strings.Fields(line)[0])
		if name == "" || name == "." || name == ".." {
			continue
		}
		names = append(names, name)
		if len(names) >= 12 {
			break
		}
	}
	if len(names) == 0 {
		return "share listed"
	}
	return "top level: " + strings.Join(names, ", ")
}

func buildStorageStatus() StorageStatus {
	s := loadSMBShare()
	st := StorageStatus{
		Name:  "Warden Storage",
		Mount: s.Mount,
		UNC:   s.UNC,
		Host:  s.Host,
	}
	st.LocalMounted = isDirMounted(s.Mount) || isDirMounted("/workspace/warden-storage")
	if st.LocalMounted {
		st.CopyReady = true
		st.Message = "Mounted on this panel at " + s.Mount + "."
	}
	used, _, avail, listing, err := probeSMBShare(s)
	if err != nil {
		if !st.LocalMounted {
			st.Message = err.Error() + " This 16G unprivileged LXC cannot kernel-mount CIFS; copies stream over SMB when login works."
		}
	} else {
		st.SMBReady = true
		st.CopyReady = true
		st.UsedBytes = used
		if used > 0 {
			st.UsedHuman = formatBytes(used)
		}
		if avail > 0 {
			st.FreeHuman = formatBytes(avail)
		}
		if st.LocalMounted {
			st.Message = "Mounted locally and reachable over SMB. " + listing
		} else {
			st.Message = "Not mounted on this 16G panel (expected). SMB to " + s.UNC + " works. Copy will stream the share to S3. " + listing
		}
	}
	backupMu.RLock()
	for _, h := range backupStore.History {
		if h.JobID == estateJobIDFull && (h.Source == s.Mount || h.Source == s.UNC || h.JobName == "Warden Storage" || h.JobName == "Shared project disk") {
			st.LastStatus = h.Status
			if h.EndTime != "" {
				st.LastTime = h.EndTime
			} else {
				st.LastTime = h.StartTime
			}
			break
		}
	}
	backupMu.RUnlock()
	return st
}

func isPlaceholderMount(path string) bool {
	switch filepath.Clean(path) {
	case "/mnt/warden-storage", "/workspace/warden-storage", "/srv/warden/storage":
		return !isDirMounted(path)
	default:
		return false
	}
}

func attachSMBProbe(st *EstateTargetStatus) {
	s := loadSMBShare()
	st.SMBUNC = s.UNC
	used, _, _, listing, err := probeSMBShare(s)
	if err != nil {
		st.Error = err.Error()
		return
	}
	st.ViaSMB = true
	st.Present = true
	st.Resolved = s.UNC
	st.Error = ""
	if listing != "" {
		st.Hint = listing
	}
	if used > 0 {
		st.SizeBytes = used
		st.SizeHuman = formatBytes(used)
		st.SizeKnown = true
	}
}

func startSMBTar(ctx context.Context, s smbShare, compress bool) (*exec.Cmd, *exec.Cmd, error) {
	if msg := s.missing(); msg != "" {
		return nil, nil, fmt.Errorf("%s", msg)
	}
	if _, err := exec.LookPath("smbclient"); err != nil {
		return nil, nil, fmt.Errorf("smbclient is not installed on this panel")
	}
	tarCmd := exec.CommandContext(ctx, "smbclient", s.UNC, "-A", s.CredFile, "-Tc", "-")
	if !compress {
		return tarCmd, nil, nil
	}
	gz := exec.CommandContext(ctx, "gzip", "-c")
	stdout, err := tarCmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	gz.Stdin = stdout
	return tarCmd, gz, nil
}
