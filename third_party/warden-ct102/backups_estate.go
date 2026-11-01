package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	estateJobIDFull      = "estate-full"
	estateTargetStorage  = "warden-storage"
	estateTargetHome     = "wardenop-home"
	estateDestShippinS3  = "shippin-s3"
	estateDefaultDest    = "/var/backups/shippin"
	estateSMBHint        = "the configured SMB share"
	estateArchiveTimeout = 12 * time.Hour
	estateSizeProbeWait  = 45 * time.Second
	maxBackupEvents      = 200
)

// EstateBackupConfig is the persisted Full Backup tab state.
type EstateBackupConfig struct {
	Destination       string              `json:"destination"`
	ActiveDestination string              `json:"activeDestination"`
	Destinations      []BackupDestination `json:"destinations"`
	Compress          bool                `json:"compress"`
	AllowRootDisk     bool                `json:"allowRootDisk"`
	Targets           []EstateTarget      `json:"targets"`
}

type BackupDestination struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"` // local, s3
	Provider     string `json:"provider,omitempty"`
	Path         string `json:"path,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	Region       string `json:"region,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	Prefix       string `json:"prefix,omitempty"`
	AccessKeyEnv string `json:"accessKeyEnv,omitempty"`
	SecretKeyEnv string `json:"secretKeyEnv,omitempty"`
}

type EstateTarget struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Source  string `json:"source"`
	Enabled bool   `json:"enabled"`
	Hint    string `json:"hint,omitempty"`
}

type EstateTargetStatus struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Source     string   `json:"source"`
	Resolved   string   `json:"resolved"`
	Hint       string   `json:"hint"`
	Enabled    bool     `json:"enabled"`
	Present    bool     `json:"present"`
	SizeBytes  uint64   `json:"sizeBytes"`
	SizeHuman  string   `json:"sizeHuman"`
	SizeKnown  bool     `json:"sizeKnown"`
	Error      string   `json:"error,omitempty"`
	Candidates []string `json:"candidates"`
	IsStorage  bool     `json:"isStorage"`
	ViaSMB     bool     `json:"viaSmb,omitempty"`
	SMBUNC     string   `json:"smbUnc,omitempty"`
}

type DestStatus struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Provider       string `json:"provider,omitempty"`
	Path           string `json:"path"`
	Parent         string `json:"parent"`
	Endpoint       string `json:"endpoint,omitempty"`
	Region         string `json:"region,omitempty"`
	Bucket         string `json:"bucket,omitempty"`
	Prefix         string `json:"prefix,omitempty"`
	URI            string `json:"uri,omitempty"`
	Present        bool   `json:"present"`
	CredsReady     bool   `json:"credsReady"`
	CredsSource    string `json:"credsSource,omitempty"`
	HasAccessKey   bool   `json:"hasAccessKey"`
	HasSecretKey   bool   `json:"hasSecretKey"`
	FreeBytes      uint64 `json:"freeBytes"`
	FreeHuman      string `json:"freeHuman"`
	RootTotalBytes uint64 `json:"rootTotalBytes"`
	RootTotalHuman string `json:"rootTotalHuman"`
	OnRootDisk     bool   `json:"onRootDisk"`
	LimitedRoot    bool   `json:"limitedRoot"`
	Error          string `json:"error,omitempty"`
}

type BackupProgress struct {
	RunID        string `json:"runId"`
	JobID        string `json:"jobId"`
	JobName      string `json:"jobName"`
	Status       string `json:"status"`
	Phase        string `json:"phase"`
	TargetID     string `json:"targetId"`
	TargetName   string `json:"targetName"`
	Source       string `json:"source"`
	Dest         string `json:"dest"`
	DestKind     string `json:"destKind,omitempty"`
	ObjectKey    string `json:"objectKey,omitempty"`
	FilesSeen    int    `json:"filesSeen"`
	BytesWritten uint64 `json:"bytesWritten"`
	SizeHuman    string `json:"sizeHuman"`
	LastLine     string `json:"lastLine"`
	StartedAt    string `json:"startedAt"`
	Error        string `json:"error,omitempty"`
}

type EstatePreflight struct {
	Destination     DestStatus           `json:"destination"`
	Destinations    []DestStatus         `json:"destinations"`
	ActiveDestID    string               `json:"activeDestId"`
	Targets         []EstateTargetStatus `json:"targets"`
	Ready           bool                 `json:"ready"`
	Blocking        []string             `json:"blocking"`
	Warnings        []string             `json:"warnings"`
	TotalBytes      uint64               `json:"totalBytes"`
	TotalHuman      string               `json:"totalHuman"`
	Running         bool                 `json:"running"`
	Progress        *BackupProgress      `json:"progress,omitempty"`
	AllowRootDisk   bool                 `json:"allowRootDisk"`
	Compress        bool                 `json:"compress"`
	LastSuccess     string               `json:"lastSuccess,omitempty"`
	Infisical       infisicalS3Status    `json:"infisical"`
	Checks          []BackupCheck        `json:"checks"`
	Events          []BackupEvent        `json:"events"`
	S3Ping          string               `json:"s3Ping,omitempty"`
	Coverage        []CoverageItem       `json:"coverage"`
	CoverageSummary CoverageSummary      `json:"coverageSummary"`
	Storage         StorageStatus        `json:"storage"`
}

type backupManifest struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	TargetID   string `json:"targetId"`
	TargetName string `json:"targetName"`
	Source     string `json:"source"`
	Archive    string `json:"archive"`
	SHA256     string `json:"sha256"`
	SizeBytes  uint64 `json:"sizeBytes"`
	FileCount  int    `json:"fileCount"`
	Hostname   string `json:"hostname"`
	Version    string `json:"version"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
}

var (
	progressMu   sync.RWMutex
	liveProgress *BackupProgress
)

func defaultEstateDestination() string {
	if p := strings.TrimSpace(os.Getenv("WARDEN_BACKUP_DEST")); p != "" {
		return p
	}
	return estateDefaultDest
}

func defaultEstateTargets() []EstateTarget {
	storageSrc := strings.TrimSpace(os.Getenv("WARDEN_STORAGE_PATH"))
	if storageSrc == "" {
		storageSrc = strings.TrimSpace(os.Getenv("WARDEN_SHARED_STORAGE_01_LINUX_MOUNT"))
	}
	if storageSrc == "" {
		storageSrc = firstExistingPath(storageCandidates(""))
	}
	if storageSrc == "" {
		storageSrc = smbDefaultMount
	}
	homeSrc := strings.TrimSpace(os.Getenv("WARDEN_OPERATOR_HOME"))
	if homeSrc == "" {
		homeSrc = "/home/wardenop"
	}
	return []EstateTarget{
		{
			ID:      "warden-config",
			Name:    "Warden config (this machine)",
			Source:  "/etc/warden",
			Enabled: true,
			Hint:    "Always on this host. Use this first to prove copies reach the bucket.",
		},
		{
			ID:      estateTargetStorage,
			Name:    "Warden Storage",
			Source:  storageSrc,
			Enabled: true,
			Hint:    "Main backup. 400G share on LXC 117 (" + estateSMBHint + "). This 16G panel streams it over SMB; it will not fit on the system disk.",
		},
		{
			ID:      estateTargetHome,
			Name:    "Operator home",
			Source:  homeSrc,
			Enabled: false,
			Hint:    "Turn on after /home/wardenop exists. Disabled so a missing path cannot block the first copy.",
		},
	}
}

func storageCandidates(configured string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add(configured)
	add(os.Getenv("WARDEN_STORAGE_PATH"))
	add("/srv/warden/storage")
	add("/workspace/warden-storage")
	add("/mnt/warden/storage")
	add("/mnt/warden-storage")
	return out
}

func homeCandidates(configured string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add(configured)
	add(os.Getenv("WARDEN_OPERATOR_HOME"))
	add("/home/wardenop")
	return out
}

func firstExistingPath(paths []string) string {
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return ""
}

func defaultShippinS3() BackupDestination {
	endpoint := s3EndpointEnv()
	region := s3RegionEnv()
	if region == "" {
		region = "us-east-1"
	}
	prefix := s3PrefixEnv()
	if prefix == "" {
		prefix = "shippin/estate"
	}
	return BackupDestination{
		ID:           estateDestShippinS3,
		Name:         "shippinS3 storage",
		Kind:         "s3",
		Provider:     "s3",
		Endpoint:     endpoint,
		Region:       region,
		Bucket:       s3BucketEnv(),
		Prefix:       prefix,
		AccessKeyEnv: "SHIPPIN_S3_ACCESS_KEY",
		SecretKeyEnv: "SHIPPIN_S3_SECRET_KEY",
	}
}

func seedEstateDefaultsLocked() bool {
	changed := false
	if backupStore.Estate.Destination == "" {
		backupStore.Estate.Destination = defaultEstateDestination()
		changed = true
	}
	if len(backupStore.Estate.Targets) == 0 {
		backupStore.Estate.Targets = defaultEstateTargets()
		backupStore.Estate.Compress = true
		changed = true
	}
	hasS3 := false
	for _, d := range backupStore.Estate.Destinations {
		if d.ID == estateDestShippinS3 {
			hasS3 = true
			break
		}
	}
	if !hasS3 {
		backupStore.Estate.Destinations = append([]BackupDestination{defaultShippinS3()}, backupStore.Estate.Destinations...)
		changed = true
	}
	if backupStore.Estate.ActiveDestination == "" {
		backupStore.Estate.ActiveDestination = estateDestShippinS3
		changed = true
	}
	hasProof := false
	for _, t := range backupStore.Estate.Targets {
		if t.ID == "warden-config" || t.Source == "/etc/warden" {
			hasProof = true
			break
		}
	}
	if !hasProof {
		proof := EstateTarget{
			ID:      "warden-config",
			Name:    "Warden config (this machine)",
			Source:  "/etc/warden",
			Enabled: true,
			Hint:    "Always on this host. Use this first to prove copies reach the bucket.",
		}
		backupStore.Estate.Targets = append([]EstateTarget{proof}, backupStore.Estate.Targets...)
		changed = true
	}
	return changed
}

func activeDestinationLocked() BackupDestination {
	id := backupStore.Estate.ActiveDestination
	for _, d := range backupStore.Estate.Destinations {
		if d.ID == id {
			return d
		}
	}
	if len(backupStore.Estate.Destinations) > 0 {
		return backupStore.Estate.Destinations[0]
	}
	return defaultShippinS3()
}

func diskFreeBytes(path string) (uint64, error) {
	var s syscallStatfs
	if err := statfs(path, &s); err != nil {
		return 0, err
	}
	if s.Bsize <= 0 {
		return 0, fmt.Errorf("invalid block size")
	}
	return s.Bavail * uint64(s.Bsize), nil
}

func diskTotalBytes(path string) (uint64, error) {
	var s syscallStatfs
	if err := statfs(path, &s); err != nil {
		return 0, err
	}
	if s.Bsize <= 0 {
		return 0, fmt.Errorf("invalid block size")
	}
	return s.Blocks * uint64(s.Bsize), nil
}

func destOnRootDisk(path string) bool {
	probe := path
	if _, err := os.Stat(probe); err != nil {
		probe = filepath.Dir(path)
	}
	a, err1 := filesystemDev("/")
	b, err2 := filesystemDev(probe)
	if err1 != nil || err2 != nil {
		return false
	}
	return a == b
}

func copyProgress(p *BackupProgress) *BackupProgress {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func currentProgress() *BackupProgress {
	progressMu.RLock()
	defer progressMu.RUnlock()
	return copyProgress(liveProgress)
}

func setProgress(p *BackupProgress) {
	progressMu.Lock()
	liveProgress = copyProgress(p)
	progressMu.Unlock()
}

func updateProgress(fn func(*BackupProgress)) {
	progressMu.Lock()
	if liveProgress != nil {
		fn(liveProgress)
	}
	progressMu.Unlock()
}

func isJobRunning(id string) bool {
	backupMu.RLock()
	defer backupMu.RUnlock()
	return runningJobs[id]
}

func markJobRunning(id string) bool {
	backupMu.Lock()
	defer backupMu.Unlock()
	if runningJobs[id] {
		return false
	}
	runningJobs[id] = true
	return true
}

func unmarkJobRunning(id string) {
	backupMu.Lock()
	delete(runningJobs, id)
	backupMu.Unlock()
}

func lastEstateSuccessLocked() string {
	for _, run := range backupStore.History {
		if run.Kind == "estate" && run.Status == "success" {
			return run.EndTime
		}
	}
	return ""
}

func buildEstatePreflight(withSizes bool) EstatePreflight {
	inf := ensureInfisicalS3()
	backupMu.RLock()
	cfg := backupStore.Estate
	last := lastEstateSuccessLocked()
	active := activeDestinationLocked()
	backupMu.RUnlock()

	if cfg.Destination == "" {
		cfg.Destination = defaultEstateDestination()
	}
	if len(cfg.Targets) == 0 {
		cfg.Targets = defaultEstateTargets()
		cfg.Compress = true
	}
	if len(cfg.Destinations) == 0 {
		cfg.Destinations = []BackupDestination{defaultShippinS3()}
		active = cfg.Destinations[0]
	}

	pf := EstatePreflight{
		AllowRootDisk: cfg.AllowRootDisk,
		Compress:      cfg.Compress,
		Running:       isJobRunning(estateJobIDFull),
		Progress:      currentProgress(),
		LastSuccess:   last,
		ActiveDestID:  active.ID,
		Infisical:     inf,
	}
	for _, d := range cfg.Destinations {
		st := probeBackupDestination(d, inf)
		pf.Destinations = append(pf.Destinations, st)
		if d.ID == active.ID {
			pf.Destination = st
		}
	}
	if pf.Destination.ID == "" {
		pf.Destination = probeBackupDestination(active, inf)
	}

	for _, t := range cfg.Targets {
		st := probeEstateTarget(t, withSizes && t.Enabled)
		pf.Targets = append(pf.Targets, st)
		if st.Enabled && st.Present && st.SizeKnown {
			pf.TotalBytes += st.SizeBytes
		}
	}
	if pf.TotalBytes > 0 {
		pf.TotalHuman = formatBytes(pf.TotalBytes)
	}

	present := 0
	storagePresent := false
	for _, t := range pf.Targets {
		if t.Enabled && t.Present {
			present++
			if t.IsStorage {
				storagePresent = true
			}
		} else if t.Enabled && !t.Present {
			pf.Warnings = append(pf.Warnings, t.Name+" is not present at "+t.Source)
		}
	}

	if present == 0 {
		pf.Blocking = append(pf.Blocking, "Nothing to copy yet: no enabled folder exists on this machine. Add /etc/warden (Warden config) or click Mount shared storage.")
	}
	if pf.Destination.Error != "" {
		pf.Blocking = append(pf.Blocking, pf.Destination.Error)
	}
	if pf.Destination.Kind == "s3" && !pf.Destination.CredsReady {
		pf.Blocking = append(pf.Blocking, "Enter the SystemFreaks access key, secret key, and bucket on this page, then Save settings")
	}
	if pf.Destination.Kind != "s3" && pf.Destination.OnRootDisk {
		pf.Warnings = append(pf.Warnings, "Destination is on this LXC's system disk ("+pf.Destination.RootTotalHuman+"). warden-storage will not fit here.")
		if storagePresent && !cfg.AllowRootDisk {
			pf.Blocking = append(pf.Blocking, "Refuse to write warden-storage onto the system disk. Use shippinS3 storage or a larger disk.")
		}
	}
	if pf.Destination.LimitedRoot && pf.Destination.OnRootDisk && pf.Destination.Kind != "s3" {
		pf.Warnings = append(pf.Warnings, "This host is the 16G warden LXC. Stream the archive to SystemFreaks S3 instead of /.")
	}
	if pf.Destination.Kind != "s3" && withSizes && pf.TotalBytes > 0 && pf.Destination.FreeBytes > 0 {
		need := pf.TotalBytes + pf.TotalBytes/10
		if pf.Destination.FreeBytes < need {
			pf.Blocking = append(pf.Blocking, fmt.Sprintf("Destination has %s free; need about %s", pf.Destination.FreeHuman, formatBytes(need)))
		}
	}
	if pf.Running {
		pf.Warnings = append(pf.Warnings, "A full backup is already running")
	}

	pf.Storage = buildStorageStatus()
	storageEnabled := false
	storageReady := false
	for _, t := range pf.Targets {
		if !t.IsStorage {
			continue
		}
		if t.Enabled {
			storageEnabled = true
			if t.Present || t.ViaSMB {
				storageReady = true
			}
		}
	}
	if storageEnabled && !storageReady {
		pf.Blocking = append(pf.Blocking, "Warden Storage is the main backup and is not reachable. It lives on LXC 117 as "+estateSMBHint+". This panel streams it over SMB; kernel mount is not required.")
	}

	pf.Ready = len(pf.Blocking) == 0 && present > 0 && !pf.Running
	if storageEnabled && storageReady {
		pf.Ready = len(pf.Blocking) == 0 && !pf.Running
	}
	pf.Coverage = buildCoverage()
	pf.CoverageSummary = summarizeCoverage(pf.Coverage)
	pf.Events = recentBackupEvents(40)
	buildBackupChecks(&pf)
	return pf
}

func probeBackupDestination(d BackupDestination, inf infisicalS3Status) DestStatus {
	if d.Kind == "s3" {
		return probeS3Destination(d, inf)
	}
	path := d.Path
	if path == "" {
		path = backupStore.Estate.Destination
	}
	ds := probeLocalDestination(path)
	ds.ID = d.ID
	ds.Name = d.Name
	ds.Kind = "local"
	return ds
}

func probeS3Destination(d BackupDestination, inf infisicalS3Status) DestStatus {
	if d.Endpoint == "" {
		d.Endpoint = s3EndpointEnv()
	}
	if d.Region == "" {
		d.Region = s3RegionEnv()
		if d.Region == "" {
			d.Region = "us-east-1"
		}
	}
	if d.Bucket == "" {
		d.Bucket = s3BucketEnv()
	}
	if d.Prefix == "" {
		d.Prefix = s3PrefixEnv()
		if d.Prefix == "" {
			d.Prefix = "shippin/estate"
		}
	}
	access := destAccessKey(d.ID)
	secret := destSecretKey(d.ID)
	source := "missing"
	switch {
	case destHasUIKeys(d.ID):
		source = "ui"
	case inf.Ready:
		source = inf.Source
		if source == "" {
			source = "env"
		}
	}
	ds := DestStatus{
		ID:           d.ID,
		Name:         d.Name,
		Kind:         "s3",
		Provider:     d.Provider,
		Endpoint:     d.Endpoint,
		Region:       d.Region,
		Bucket:       d.Bucket,
		Prefix:       d.Prefix,
		HasAccessKey: access != "",
		HasSecretKey: secret != "",
		CredsReady:   access != "" && secret != "",
		CredsSource:  source,
	}
	if d.Bucket != "" {
		ds.URI = "s3://" + d.Bucket + "/" + strings.Trim(d.Prefix, "/") + "/"
		ds.Present = true
	} else {
		ds.Error = "Bucket is empty — set it on this page and Save settings"
	}
	if !ds.CredsReady {
		msg := "Access key and secret key are not saved yet"
		if ds.Error != "" {
			ds.Error += "; " + msg
		} else {
			ds.Error = msg
		}
	}
	return ds
}

func probeLocalDestination(path string) DestStatus {
	path = filepath.Clean(strings.TrimSpace(path))
	ds := DestStatus{Path: path, Parent: filepath.Dir(path), Kind: "local"}
	if tot, err := diskTotalBytes("/"); err == nil {
		ds.RootTotalBytes = tot
		ds.RootTotalHuman = formatBytes(tot)
		ds.LimitedRoot = tot > 0 && tot < 32<<30
	}
	probe := path
	if st, err := os.Stat(path); err == nil {
		ds.Present = st.IsDir()
		if !st.IsDir() {
			ds.Error = "destination exists but is not a directory"
			return ds
		}
	} else {
		probe = filepath.Dir(path)
		if _, err := os.Stat(probe); err != nil {
			ds.Error = "destination parent does not exist: " + probe
			return ds
		}
	}
	if free, err := diskFreeBytes(probe); err == nil {
		ds.FreeBytes = free
		ds.FreeHuman = formatBytes(free)
	} else {
		ds.Error = "cannot read destination free space: " + err.Error()
	}
	ds.OnRootDisk = destOnRootDisk(probe)
	return ds
}

func probeDestination(path string) DestStatus {
	return probeLocalDestination(path)
}

func probeEstateTarget(t EstateTarget, withSize bool) EstateTargetStatus {
	st := EstateTargetStatus{
		ID:        t.ID,
		Name:      t.Name,
		Source:    t.Source,
		Hint:      t.Hint,
		Enabled:   t.Enabled,
		IsStorage: t.ID == estateTargetStorage,
	}
	if t.ID == estateTargetStorage {
		st.Candidates = storageCandidates(t.Source)
	} else if t.ID == estateTargetHome {
		st.Candidates = homeCandidates(t.Source)
	} else if t.Source != "" {
		st.Candidates = []string{t.Source}
	}

	resolved := firstExistingPath(st.Candidates)
	if st.IsStorage && resolved != "" && isPlaceholderMount(resolved) {
		resolved = ""
	}
	if resolved == "" {
		resolved = t.Source
	}
	st.Resolved = resolved
	info, err := os.Stat(resolved)
	if err != nil || (st.IsStorage && isPlaceholderMount(resolved)) {
		if st.IsStorage {
			attachSMBProbe(&st)
			if st.ViaSMB {
				return st
			}
		}
		st.Error = "not present"
		return st
	}
	st.Present = true
	st.Source = resolved
	if !info.IsDir() {
		st.SizeBytes = uint64(info.Size())
		st.SizeHuman = formatBytes(st.SizeBytes)
		st.SizeKnown = true
		return st
	}
	if !withSize {
		return st
	}
	size, err := dirSizeBytes(resolved)
	if err != nil {
		st.Error = "present, size unknown: " + err.Error()
		return st
	}
	st.SizeBytes = size
	st.SizeHuman = formatBytes(size)
	st.SizeKnown = true
	return st
}

func dirSizeBytes(path string) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), estateSizeProbeWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, "du", "-sb", path)
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return 0, fmt.Errorf("size probe timed out")
	}
	if err != nil {
		return 0, err
	}
	field := strings.Fields(string(out))
	if len(field) == 0 {
		return 0, fmt.Errorf("empty du output")
	}
	n, err := strconv.ParseUint(field[0], 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func executeEstateBackup(targetIDs []string) {
	if !markJobRunning(estateJobIDFull) {
		log.Printf("Estate backup already running")
		return
	}
	defer unmarkJobRunning(estateJobIDFull)

	pf := buildEstatePreflight(true)
	if len(pf.Blocking) > 0 {
		msg := strings.Join(pf.Blocking, "; ")
		log.Printf("Estate backup blocked: %s", msg)
		appendBackupEvent("error", "run", "Blocked: "+msg)
		failEstateRun(msg)
		return
	}

	dest := pf.Destination
	if dest.Kind != "s3" {
		if err := os.MkdirAll(dest.Path, 0755); err != nil {
			failEstateRun("create destination: " + err.Error())
			return
		}
	}

	var selected []EstateTargetStatus
	want := map[string]bool{}
	for _, id := range targetIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = true
		}
	}
	fullRun := len(want) == 0
	for _, t := range pf.Targets {
		if len(want) > 0 && !want[t.ID] {
			continue
		}
		if !t.Enabled {
			continue
		}
		copyable := t.Present || t.ViaSMB
		if t.IsStorage && !copyable {
			msg := "Warden Storage is the main backup and is not reachable over SMB (" + estateSMBHint + "). Refusing to call this a backup."
			appendBackupEvent("error", "run", msg)
			failEstateRun(msg)
			return
		}
		if !copyable {
			appendBackupEvent("warn", "run", "Skipped "+t.Name+" — not on this machine ("+t.Source+")")
			continue
		}
		selected = append(selected, t)
	}
	if fullRun {
		var storage, rest []EstateTargetStatus
		for _, t := range selected {
			if t.IsStorage {
				storage = append(storage, t)
			} else {
				rest = append(rest, t)
			}
		}
		selected = append(storage, rest...)
	}
	if len(selected) == 0 {
		appendBackupEvent("error", "run", "No enabled folder exists on this machine")
		failEstateRun("no present targets selected")
		return
	}

	overallID := fmt.Sprintf("%d", time.Now().UnixNano())
	started := time.Now()
	destLabel := dest.Path
	if dest.Kind == "s3" {
		destLabel = dest.URI
	}
	setProgress(&BackupProgress{
		RunID:     overallID,
		JobID:     estateJobIDFull,
		JobName:   "Full estate backup",
		Status:    "running",
		Phase:     "starting",
		Dest:      destLabel,
		DestKind:  dest.Kind,
		StartedAt: started.Format("2006-01-02 15:04:05"),
	})

	appendBackupEvent("info", "run", fmt.Sprintf("Copying %d item(s) to %s", len(selected), destLabel))
	var failed error
	for _, t := range selected {
		appendBackupEvent("info", "run", "Copying "+t.Name+" from "+t.Source)
		if err := archiveEstateTarget(overallID, dest, t, pf.Compress); err != nil {
			failed = err
			log.Printf("Estate backup failed [%s]: %v", t.Name, err)
			appendBackupEvent("error", "run", "Failed "+t.Name+": "+err.Error())
			break
		}
		appendBackupEvent("ok", "run", "Copied "+t.Name)
	}

	if failed != nil {
		updateProgress(func(p *BackupProgress) {
			p.Status = "failed"
			p.Phase = "failed"
			p.Error = failed.Error()
		})
		appendBackupEvent("error", "run", "Backup failed: "+failed.Error())
		return
	}
	updateProgress(func(p *BackupProgress) {
		p.Status = "success"
		p.Phase = "done"
		p.LastLine = "Full backup complete"
	})
	appendBackupEvent("ok", "run", "Backup finished in "+formatDuration(time.Since(started)))
	log.Printf("Estate backup completed in %s", formatDuration(time.Since(started)))
}

func failEstateRun(msg string) {
	run := BackupRun{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		JobID:     estateJobIDFull,
		JobName:   "Full estate backup",
		Kind:      "estate",
		Type:      "tar",
		Status:    "failed",
		StartTime: time.Now().Format("2006-01-02 15:04:05"),
		EndTime:   time.Now().Format("2006-01-02 15:04:05"),
		Error:     msg,
	}
	appendBackupRun(run)
	setProgress(&BackupProgress{
		RunID:     run.ID,
		JobID:     estateJobIDFull,
		JobName:   run.JobName,
		Status:    "failed",
		Phase:     "failed",
		Error:     msg,
		StartedAt: run.StartTime,
	})
}

func archiveEstateTarget(parentID string, dest DestStatus, t EstateTargetStatus, compress bool) error {
	start := time.Now()
	destLabel := dest.Path
	if dest.Kind == "s3" {
		destLabel = dest.URI
	}
	run := BackupRun{
		ID:        fmt.Sprintf("%s-%s", parentID, t.ID),
		JobID:     estateJobIDFull,
		JobName:   t.Name,
		Kind:      "estate",
		Type:      "tar",
		Source:    t.Source,
		Dest:      destLabel,
		Status:    "running",
		StartTime: start.Format("2006-01-02 15:04:05"),
	}
	appendBackupRun(run)
	setProgress(&BackupProgress{
		RunID:      run.ID,
		JobID:      estateJobIDFull,
		JobName:    t.Name,
		Status:     "running",
		Phase:      "archiving",
		TargetID:   t.ID,
		TargetName: t.Name,
		Source:     t.Source,
		Dest:       destLabel,
		DestKind:   dest.Kind,
		StartedAt:  run.StartTime,
	})

	stamp := start.Format("20060102-150405")
	base := "shippin-" + t.ID + "-" + stamp
	if dest.Kind == "s3" {
		return archiveEstateTargetS3(run.ID, dest, t, base, compress)
	}

	archive, count, err := archiveDirectory(t.Source, dest.Path, base, compress, t.ID)
	if err != nil {
		finishEstateRun(run.ID, "failed", "", "", 0, count, err)
		return err
	}

	updateProgress(func(p *BackupProgress) {
		p.Phase = "hashing"
		p.LastLine = "SHA-256 " + filepath.Base(archive)
		if info, serr := os.Stat(archive); serr == nil {
			p.BytesWritten = uint64(info.Size())
			p.SizeHuman = formatBytes(p.BytesWritten)
		}
	})

	sum, err := sha256File(archive)
	if err != nil {
		finishEstateRun(run.ID, "failed", archive, "", 0, count, err)
		return err
	}

	info, _ := os.Stat(archive)
	var size uint64
	if info != nil {
		size = uint64(info.Size())
	}
	host, _ := os.Hostname()
	man := backupManifest{
		ID:         run.ID,
		Kind:       "estate",
		TargetID:   t.ID,
		TargetName: t.Name,
		Source:     t.Source,
		Archive:    archive,
		SHA256:     sum,
		SizeBytes:  size,
		FileCount:  count,
		Hostname:   host,
		Version:    Version,
		StartedAt:  run.StartTime,
		FinishedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	manPath := archive + ".manifest.json"
	data, _ := json.MarshalIndent(man, "", "  ")
	if werr := os.WriteFile(manPath, data, 0600); werr != nil {
		log.Printf("Failed to write backup manifest: %v", werr)
		manPath = ""
	}
	_ = os.WriteFile(archive+".sha256", []byte(sum+"  "+filepath.Base(archive)+"\n"), 0600)

	finishEstateRun(run.ID, "success", archive, manPath, size, count, nil)
	backupMu.Lock()
	for i := range backupStore.History {
		if backupStore.History[i].ID == run.ID {
			backupStore.History[i].SHA256 = sum
			break
		}
	}
	backupMu.Unlock()
	saveBackupStore()
	return nil
}

func startArchiveStream(ctx context.Context, t EstateTargetStatus, compress bool) (io.ReadCloser, func() error, error) {
	useSMB := t.ViaSMB || (t.IsStorage && !fileExists(t.Source))
	if useSMB {
		share := loadSMBShare()
		tarCmd, gzCmd, err := startSMBTar(ctx, share, compress)
		if err != nil {
			return nil, nil, err
		}
		if gzCmd != nil {
			out, err := gzCmd.StdoutPipe()
			if err != nil {
				return nil, nil, err
			}
			if err := tarCmd.Start(); err != nil {
				return nil, nil, err
			}
			if err := gzCmd.Start(); err != nil {
				_ = tarCmd.Wait()
				return nil, nil, err
			}
			return out, func() error {
				gzErr := gzCmd.Wait()
				tarErr := tarCmd.Wait()
				if gzErr != nil {
					return gzErr
				}
				return tarErr
			}, nil
		}
		out, err := tarCmd.StdoutPipe()
		if err != nil {
			return nil, nil, err
		}
		if err := tarCmd.Start(); err != nil {
			return nil, nil, err
		}
		return out, tarCmd.Wait, nil
	}

	source := filepath.Clean(t.Source)
	parent := filepath.Dir(source)
	name := filepath.Base(source)
	args := []string{"--numeric-owner", "-cvf", "-"}
	if compress {
		args = []string{"--numeric-owner", "-cvzf", "-"}
	}
	args = append(args, "-C", parent, name)
	cmd := exec.CommandContext(ctx, "tar", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		n := 0
		for scanner.Scan() {
			n++
			line := scanner.Text()
			if n%50 == 0 || n == 1 {
				updateProgress(func(p *BackupProgress) {
					if p.TargetID != t.ID {
						return
					}
					p.FilesSeen = n
					p.LastLine = line
				})
			}
		}
	}()
	return stdout, cmd.Wait, nil
}

func archiveEstateTargetS3(runID string, dest DestStatus, t EstateTargetStatus, base string, compress bool) error {
	client, err := newS3Client(dest.Endpoint, dest.Region, dest.Bucket, destAccessKey(dest.ID), destSecretKey(dest.ID))
	if err != nil {
		finishEstateRun(runID, "failed", "", "", 0, 0, err)
		return err
	}
	ext := ".tar"
	if compress {
		ext = ".tar.gz"
	}
	filename := base + ext
	key := joinS3Key(dest.Prefix, filename)
	updateProgress(func(p *BackupProgress) {
		p.Phase = "uploading"
		p.ObjectKey = key
		p.LastLine = "s3://" + dest.Bucket + "/" + key
	})

	ctx, cancel := context.WithTimeout(context.Background(), estateArchiveTimeout)
	defer cancel()

	stdout, wait, err := startArchiveStream(ctx, t, compress)
	if err != nil {
		finishEstateRun(runID, "failed", "", "", 0, 0, err)
		return err
	}

	count := 0
	hasher := sha256.New()
	reader := io.TeeReader(stdout, hasher)
	size, err := client.streamUpload(key, reader, func(n uint64) {
		updateProgress(func(p *BackupProgress) {
			if p.TargetID != t.ID {
				return
			}
			p.BytesWritten = n
			p.SizeHuman = formatBytes(n)
			p.Phase = "uploading"
		})
	})
	waitErr := wait()
	if err != nil {
		finishEstateRun(runID, "failed", "", "", size, count, err)
		return err
	}
	if ctx.Err() == context.DeadlineExceeded {
		err := fmt.Errorf("archive timed out after %s", estateArchiveTimeout)
		finishEstateRun(runID, "failed", "", "", size, count, err)
		return err
	}
	if waitErr != nil {
		finishEstateRun(runID, "failed", "", "", size, count, waitErr)
		return waitErr
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	uri := s3ObjectURI(dest.Endpoint, dest.Bucket, key)
	host, _ := os.Hostname()
	man := backupManifest{
		ID:         runID,
		Kind:       "estate",
		TargetID:   t.ID,
		TargetName: t.Name,
		Source:     t.Source,
		Archive:    uri,
		SHA256:     sum,
		SizeBytes:  size,
		FileCount:  count,
		Hostname:   host,
		Version:    Version,
		StartedAt:  time.Now().Format("2006-01-02 15:04:05"),
		FinishedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	manBytes, _ := json.MarshalIndent(man, "", "  ")
	manKey := key + ".manifest.json"
	if werr := client.putObject(manKey, manBytes, "application/json"); werr != nil {
		log.Printf("Failed to write S3 backup manifest: %v", werr)
		manKey = ""
	} else {
		_ = client.putObject(key+".sha256", []byte(sum+"  "+filename+"\n"), "text/plain")
	}

	finishEstateRun(runID, "success", uri, s3ObjectURI(dest.Endpoint, dest.Bucket, manKey), size, count, nil)
	backupMu.Lock()
	for i := range backupStore.History {
		if backupStore.History[i].ID == runID {
			backupStore.History[i].SHA256 = sum
			break
		}
	}
	backupMu.Unlock()
	saveBackupStore()
	updateProgress(func(p *BackupProgress) {
		p.ObjectKey = key
		p.LastLine = uri
	})
	return nil
}

func finishEstateRun(id, status, filePath, manifest string, size uint64, files int, err error) {
	end := time.Now()
	backupMu.Lock()
	var start time.Time
	for i := range backupStore.History {
		if backupStore.History[i].ID == id {
			backupStore.History[i].Status = status
			backupStore.History[i].EndTime = end.Format("2006-01-02 15:04:05")
			backupStore.History[i].FilePath = filePath
			backupStore.History[i].ManifestPath = manifest
			backupStore.History[i].FileCount = files
			if size > 0 {
				backupStore.History[i].Size = formatBytes(size)
			}
			if parsed, perr := time.Parse("2006-01-02 15:04:05", backupStore.History[i].StartTime); perr == nil {
				start = parsed
				backupStore.History[i].Duration = formatDuration(end.Sub(parsed))
			}
			if err != nil {
				backupStore.History[i].Error = err.Error()
			}
			break
		}
	}
	backupMu.Unlock()
	saveBackupStore()
	_ = start
	updateProgress(func(p *BackupProgress) {
		if p.RunID != id {
			return
		}
		p.Status = status
		p.FilesSeen = files
		p.BytesWritten = size
		if size > 0 {
			p.SizeHuman = formatBytes(size)
		}
		if err != nil {
			p.Error = err.Error()
			p.Phase = "failed"
		} else {
			p.Phase = "done"
			p.LastLine = filepath.Base(filePath)
		}
	})
}

func appendBackupRun(run BackupRun) {
	backupMu.Lock()
	backupStore.History = append([]BackupRun{run}, backupStore.History...)
	if len(backupStore.History) > maxBackupHistory {
		backupStore.History = backupStore.History[:maxBackupHistory]
	}
	backupMu.Unlock()
	saveBackupStore()
}

func archiveDirectory(source, destDir, base string, compress bool, targetID string) (string, int, error) {
	source = filepath.Clean(source)
	info, err := os.Stat(source)
	if err != nil {
		return "", 0, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("source is not a file or directory")
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", 0, err
	}

	filename := base + ".tar"
	args := []string{"--numeric-owner", "-cvf"}
	if compress {
		filename = base + ".tar.gz"
		args = []string{"--numeric-owner", "-cvzf"}
	}
	archive := filepath.Join(destDir, filename)
	args = append(args, archive)

	absDest, _ := filepath.Abs(destDir)
	absSrc, _ := filepath.Abs(source)
	if strings.HasPrefix(absDest, absSrc+string(os.PathSeparator)) {
		rel, rerr := filepath.Rel(absSrc, absDest)
		if rerr == nil {
			args = append(args, "--exclude="+rel)
		}
	}

	parent := filepath.Dir(source)
	name := filepath.Base(source)
	args = append(args, "-C", parent, name)

	ctx, cancel := context.WithTimeout(context.Background(), estateArchiveTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tar", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", 0, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", 0, err
	}

	done := make(chan struct{})
	go func() {
		tick := time.NewTicker(800 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				if info, err := os.Stat(archive); err == nil {
					sz := uint64(info.Size())
					updateProgress(func(p *BackupProgress) {
						if p.TargetID != targetID {
							return
						}
						p.BytesWritten = sz
						p.SizeHuman = formatBytes(sz)
					})
				}
			}
		}
	}()

	count := 0
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		count++
		if count%50 == 0 || count == 1 {
			updateProgress(func(p *BackupProgress) {
				if p.TargetID != targetID {
					return
				}
				p.FilesSeen = count
				p.LastLine = line
			})
		}
	}
	waitErr := cmd.Wait()
	close(done)
	if ctx.Err() == context.DeadlineExceeded {
		os.Remove(archive)
		return "", count, fmt.Errorf("archive timed out after %s", estateArchiveTimeout)
	}
	if waitErr != nil {
		os.Remove(archive)
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return "", count, fmt.Errorf("tar: %s", msg)
	}
	return archive, count, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
