package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func handleBackups(w http.ResponseWriter, r *http.Request) {
	backupMu.RLock()
	store := backupStore
	running := make(map[string]bool, len(runningJobs))
	for id, v := range runningJobs {
		running[id] = v
	}
	backupMu.RUnlock()

	hd := buildHeaderData(r)
	hd.PageTitle = "Backups"
	data := struct {
		HeaderData
		Jobs    []BackupJob
		History []BackupRun
		Running map[string]bool
		Estate  EstatePreflight
	}{
		HeaderData: hd,
		Jobs:       store.Jobs,
		History:    store.History,
		Running:    running,
		Estate:     buildEstatePreflight(false),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "backups.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIBackups(w http.ResponseWriter, r *http.Request) {
	sub := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/backups"), "/")
	switch sub {
	case "progress":
		handleBackupProgress(w, r)
		return
	case "preflight":
		handleBackupPreflight(w, r)
		return
	case "events":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"events":   recentBackupEvents(80),
			"running":  isJobRunning(estateJobIDFull),
			"progress": currentProgress(),
		})
		return
	}

	if r.Method == http.MethodGet {
		backupMu.RLock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(backupStore)
		backupMu.RUnlock()
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.ParseForm()
	action := r.FormValue("action")

	switch action {
	case "add":
		handleBackupAdd(w, r)
	case "edit":
		handleBackupEdit(w, r)
	case "delete":
		handleBackupDelete(w, r)
	case "toggle":
		handleBackupToggle(w, r)
	case "run_now":
		handleBackupRunNow(w, r)
	case "restore":
		handleBackupRestore(w, r)
	case "delete_run":
		handleBackupDeleteRun(w, r)
	case "clear_history":
		handleBackupClearHistory(w, r)
	case "save_estate":
		handleEstateSave(w, r)
	case "run_estate":
		handleEstateRun(w, r)
	case "add_source":
		handleEstateAddSource(w, r)
	case "edit_source":
		handleEstateEditSource(w, r)
	case "delete_source":
		handleEstateDeleteSource(w, r)
	case "move_source":
		handleEstateMoveSource(w, r)
	case "test_s3":
		handleEstateTestS3(w, r)
	case "mount_storage":
		handleEstateMountStorage(w, r)
	case "test_storage":
		handleEstateTestStorage(w, r)
	case "add_coverage":
		handleEstateAddCoverage(w, r)
	case "add_coverage_required":
		handleEstateAddCoverageRequired(w, r)
	default:
		respondJSON(w, false, "Invalid action")
	}
}

func handleBackupProgress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"running":  isJobRunning(estateJobIDFull),
		"progress": currentProgress(),
	})
}

func handleBackupPreflight(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buildEstatePreflight(true))
}

func applyEstateForm(r *http.Request) error {
	backupMu.Lock()
	defer backupMu.Unlock()
	seedEstateDefaultsLocked()

	if destID := strings.TrimSpace(r.FormValue("dest_id")); destID != "" {
		found := false
		for _, d := range backupStore.Estate.Destinations {
			if d.ID == destID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown destination")
		}
		backupStore.Estate.ActiveDestination = destID
	}

	for i := range backupStore.Estate.Destinations {
		if backupStore.Estate.Destinations[i].ID != backupStore.Estate.ActiveDestination &&
			backupStore.Estate.Destinations[i].ID != estateDestShippinS3 {
			continue
		}
		if backupStore.Estate.Destinations[i].Kind != "s3" {
			continue
		}
		if v := strings.TrimSpace(r.FormValue("s3_endpoint")); v != "" {
			backupStore.Estate.Destinations[i].Endpoint = v
		}
		if v := strings.TrimSpace(r.FormValue("s3_region")); v != "" {
			backupStore.Estate.Destinations[i].Region = v
		}
		if v := strings.TrimSpace(r.FormValue("s3_bucket")); v != "" {
			backupStore.Estate.Destinations[i].Bucket = v
		}
		if v := strings.TrimSpace(r.FormValue("s3_prefix")); v != "" {
			backupStore.Estate.Destinations[i].Prefix = v
		}
		ak := strings.TrimSpace(r.FormValue("s3_access_key"))
		sk := strings.TrimSpace(r.FormValue("s3_secret_key"))
		if ak != "" || sk != "" {
			if err := mergeDestSecrets(backupStore.Estate.Destinations[i].ID, ak, sk); err != nil {
				return err
			}
		}
	}

	if dest := strings.TrimSpace(r.FormValue("destination")); dest != "" && strings.HasPrefix(dest, "/") {
		backupStore.Estate.Destination = dest
	}

	if r.FormValue("compress") != "" {
		backupStore.Estate.Compress = r.FormValue("compress") != "off"
	}
	backupStore.Estate.AllowRootDisk = r.FormValue("allow_root_disk") == "on"
	if len(backupStore.Estate.Targets) == 0 {
		backupStore.Estate.Targets = defaultEstateTargets()
	}
	for i := range backupStore.Estate.Targets {
		id := backupStore.Estate.Targets[i].ID
		if src := strings.TrimSpace(r.FormValue("source_"+id)); src != "" {
			backupStore.Estate.Targets[i].Source = src
		}
		en := r.FormValue("enabled_" + id)
		if en != "" {
			backupStore.Estate.Targets[i].Enabled = en == "on" || en == "true" || en == "1"
		}
	}
	return nil
}

func handleEstateAddSource(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	source := strings.TrimSpace(r.FormValue("source"))
	if name == "" || source == "" {
		respondJSON(w, false, "Name and source path are required")
		return
	}
	if !strings.HasPrefix(source, "/") {
		respondJSON(w, false, "Source must be an absolute path")
		return
	}
	job := EstateTarget{
		ID:      fmt.Sprintf("src-%d", time.Now().UnixNano()),
		Name:    name,
		Source:  source,
		Enabled: true,
		Hint:    "Custom source added from the backup panel",
	}
	backupMu.Lock()
	seedEstateDefaultsLocked()
	backupStore.Estate.Targets = append(backupStore.Estate.Targets, job)
	backupMu.Unlock()
	if err := saveBackupStore(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	auditFromRequest(r, "Backup source added", name+" "+source, "system")
	appendBackupEvent("ok", "add_source", "Added folder "+name+" at "+source)
	respondJSON(w, true, "Source added")
}

func handleEstateDeleteSource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		respondJSON(w, false, "Source id is required")
		return
	}
	backupMu.Lock()
	found := false
	for i, t := range backupStore.Estate.Targets {
		if t.ID == id {
			backupStore.Estate.Targets = append(backupStore.Estate.Targets[:i], backupStore.Estate.Targets[i+1:]...)
			found = true
			break
		}
	}
	backupMu.Unlock()
	if !found {
		respondJSON(w, false, "Source not found")
		return
	}
	if err := saveBackupStore(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	auditFromRequest(r, "Backup source removed", id, "system")
	appendBackupEvent("warn", "delete_source", "Removed folder "+id)
	respondJSON(w, true, "Source removed")
}

func handleEstateEditSource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.FormValue("id"))
	name := strings.TrimSpace(r.FormValue("name"))
	source := strings.TrimSpace(r.FormValue("source"))
	if id == "" {
		respondJSON(w, false, "Source id is required")
		return
	}
	if source != "" && !strings.HasPrefix(source, "/") {
		respondJSON(w, false, "Source must be an absolute path")
		return
	}
	backupMu.Lock()
	found := false
	for i := range backupStore.Estate.Targets {
		if backupStore.Estate.Targets[i].ID != id {
			continue
		}
		if name != "" {
			backupStore.Estate.Targets[i].Name = name
		}
		if source != "" {
			backupStore.Estate.Targets[i].Source = source
		}
		en := r.FormValue("enabled")
		if en != "" {
			backupStore.Estate.Targets[i].Enabled = en == "on" || en == "true" || en == "1"
		}
		found = true
		break
	}
	backupMu.Unlock()
	if !found {
		respondJSON(w, false, "Source not found")
		return
	}
	if err := saveBackupStore(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	appendBackupEvent("ok", "edit_source", "Updated folder "+id)
	respondJSON(w, true, "Source updated")
}

func handleEstateMoveSource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.FormValue("id"))
	dir := strings.TrimSpace(r.FormValue("dir"))
	backupMu.Lock()
	idx := -1
	for i, t := range backupStore.Estate.Targets {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		backupMu.Unlock()
		respondJSON(w, false, "Source not found")
		return
	}
	swap := idx
	if dir == "up" && idx > 0 {
		swap = idx - 1
	} else if dir == "down" && idx < len(backupStore.Estate.Targets)-1 {
		swap = idx + 1
	}
	if swap != idx {
		backupStore.Estate.Targets[idx], backupStore.Estate.Targets[swap] = backupStore.Estate.Targets[swap], backupStore.Estate.Targets[idx]
	}
	backupMu.Unlock()
	if err := saveBackupStore(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	appendBackupEvent("info", "move_source", "Reordered folder "+id+" "+dir)
	respondJSON(w, true, "Order saved")
}

func handleEstateAddCoverage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		respondJSON(w, false, "Coverage id is required")
		return
	}
	var item *CoverageItem
	for _, it := range smeCoverageCatalog() {
		if it.ID == id {
			cp := it
			item = &cp
			break
		}
	}
	if item == nil {
		respondJSON(w, false, "Unknown coverage item")
		return
	}
	backupMu.Lock()
	seedEstateDefaultsLocked()
	upsertCoverageTargetLocked(*item)
	backupMu.Unlock()
	if err := saveBackupStore(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	appendBackupEvent("ok", "add_coverage", "Added to copy list: "+item.Name+" ("+item.Path+")")
	respondJSON(w, true, "Added "+item.Name+" to the copy list")
}

func handleEstateAddCoverageRequired(w http.ResponseWriter, r *http.Request) {
	n := 0
	backupMu.Lock()
	seedEstateDefaultsLocked()
	for _, it := range smeCoverageCatalog() {
		if !it.Required {
			continue
		}
		upsertCoverageTargetLocked(it)
		n++
	}
	backupMu.Unlock()
	if err := saveBackupStore(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	msg := fmt.Sprintf("Added %d required items to the copy list", n)
	appendBackupEvent("ok", "add_coverage", msg)
	respondJSON(w, true, msg+". Missing paths will be skipped and logged until they exist.")
}

func handleEstateMountStorage(w http.ResponseWriter, r *http.Request) {
	appendBackupEvent("info", "mount", "Mount shared storage requested")
	ensureInfisicalS3()
	if err := ensureWardenStorageMounted(); err != nil {
		st := buildStorageStatus()
		if st.SMBReady {
			appendBackupEvent("warn", "mount", err.Error())
			respondJSON(w, true, err.Error()+" SMB is reachable — use Copy Warden Storage. Kernel mount is not required.")
			return
		}
		appendBackupEvent("error", "mount", err.Error())
		respondJSON(w, false, err.Error())
		return
	}
	s := loadSMBShare()
	appendBackupEvent("ok", "mount", "Shared storage is mounted at "+s.Mount)
	respondJSON(w, true, "Shared storage mounted at "+s.Mount)
}

func handleEstateTestStorage(w http.ResponseWriter, r *http.Request) {
	appendBackupEvent("info", "test_storage", "Testing Warden Storage SMB")
	st := buildStorageStatus()
	if !st.CopyReady {
		appendBackupEvent("error", "test_storage", st.Message)
		respondJSON(w, false, st.Message)
		return
	}
	msg := st.Message
	if st.UsedHuman != "" {
		msg = "Warden Storage ready. About " + st.UsedHuman + " used. " + st.Message
	}
	appendBackupEvent("ok", "test_storage", msg)
	respondJSON(w, true, msg)
}

func handleEstateTestS3(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if err := applyEstateForm(r); err != nil {
		respondJSON(w, false, err.Error())
		return
	}
	_ = saveBackupStore()
	err := pingSavedS3()
	if err != nil {
		lastS3PingOK = false
		lastS3PingMsg = err.Error()
		appendBackupEvent("error", "test_s3", lastS3PingMsg)
		respondJSON(w, false, lastS3PingMsg)
		return
	}
	lastS3PingOK = true
	lastS3PingMsg = "Destination accepted a list request. Copies can be uploaded here."
	appendBackupEvent("ok", "test_s3", lastS3PingMsg)
	respondJSON(w, true, lastS3PingMsg)
}

func handleEstateSave(w http.ResponseWriter, r *http.Request) {
	if err := applyEstateForm(r); err != nil {
		appendBackupEvent("error", "save", err.Error())
		respondJSON(w, false, err.Error())
		return
	}
	if err := saveBackupStore(); err != nil {
		appendBackupEvent("error", "save", err.Error())
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	backupMu.RLock()
	d := activeDestinationLocked()
	n := len(backupStore.Estate.Targets)
	backupMu.RUnlock()
	msg := fmt.Sprintf("Saved destination %s bucket %s and %d source folder(s). Keys stored on this host.", d.Name, d.Bucket, n)
	auditFromRequest(r, "Estate backup settings saved", d.Bucket, "system")
	appendBackupEvent("ok", "save", msg)
	respondJSON(w, true, msg)
}

func handleEstateRun(w http.ResponseWriter, r *http.Request) {
	appendBackupEvent("info", "run", "Run requested")
	if isJobRunning(estateJobIDFull) {
		appendBackupEvent("warn", "run", "Refused: a backup is already running")
		respondJSON(w, false, "A full backup is already running")
		return
	}

	if err := applyEstateForm(r); err != nil && strings.TrimSpace(r.FormValue("dest_id")) != "" {
		appendBackupEvent("error", "run", err.Error())
		respondJSON(w, false, err.Error())
		return
	}
	_ = saveBackupStore()

	target := strings.TrimSpace(r.FormValue("target"))
	if err := pingSavedS3(); err != nil {
		lastS3PingOK = false
		lastS3PingMsg = err.Error()
		appendBackupEvent("error", "run", "Destination check failed: "+err.Error())
		respondJSON(w, false, "Destination check failed: "+err.Error())
		return
	}
	lastS3PingOK = true
	lastS3PingMsg = "Destination accepted a list request."

	pf := buildEstatePreflight(false)
	if target != "" {
		present := false
		for _, t := range pf.Targets {
			if t.ID == target && t.Present && t.Enabled {
				present = true
				break
			}
		}
		if !present {
			msg := "Cannot copy that folder: it is not present on this host (" + target + ")"
			appendBackupEvent("error", "run", msg)
			respondJSON(w, false, msg)
			return
		}
	} else if len(pf.Blocking) > 0 {
		msg := strings.Join(pf.Blocking, "; ")
		appendBackupEvent("error", "run", "Refused: "+msg)
		respondJSON(w, false, msg)
		return
	}

	var ids []string
	if target != "" {
		ids = []string{target}
	}

	go executeEstateBackup(ids)
	label := "all present targets"
	if target != "" {
		label = target
	}
	auditFromRequest(r, "Full estate backup started", label, "system")
	appendBackupEvent("ok", "run", "Backup started ("+label+")")
	respondJSON(w, true, "Backup started. Watch the activity log and the progress bar.")
}

func handleBackupAdd(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	bType := strings.TrimSpace(r.FormValue("type"))
	source := strings.TrimSpace(r.FormValue("source"))
	dest := strings.TrimSpace(r.FormValue("destination"))
	schedule := strings.TrimSpace(r.FormValue("schedule"))
	exclude := strings.TrimSpace(r.FormValue("exclude"))
	compress := r.FormValue("compress") == "on"

	if name == "" || source == "" || dest == "" {
		respondJSON(w, false, "Name, source, and destination are required")
		return
	}

	if bType != "tar" && bType != "rsync" {
		respondJSON(w, false, "Type must be tar or rsync")
		return
	}

	if _, err := os.Stat(source); os.IsNotExist(err) {
		respondJSON(w, false, "Source path does not exist")
		return
	}

	if schedule == "" {
		schedule = "manual"
	}

	job := BackupJob{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:        name,
		Type:        bType,
		Source:      source,
		Destination: dest,
		Schedule:    schedule,
		Compress:    compress,
		Exclude:     exclude,
		Enabled:     true,
		CreatedAt:   time.Now().Format("2006-01-02 15:04:05"),
	}

	backupMu.Lock()
	backupStore.Jobs = append(backupStore.Jobs, job)
	backupMu.Unlock()

	if err := saveBackupStore(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	auditFromRequest(r, "Backup job added", name+" ("+bType+")", "system")
	respondJSON(w, true, "Backup job added")
}

func handleBackupEdit(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	name := strings.TrimSpace(r.FormValue("name"))
	source := strings.TrimSpace(r.FormValue("source"))
	dest := strings.TrimSpace(r.FormValue("destination"))
	schedule := strings.TrimSpace(r.FormValue("schedule"))
	exclude := strings.TrimSpace(r.FormValue("exclude"))
	compress := r.FormValue("compress") == "on"

	backupMu.Lock()
	found := false
	for i := range backupStore.Jobs {
		if backupStore.Jobs[i].ID == id {
			if name != "" {
				backupStore.Jobs[i].Name = name
			}
			if source != "" {
				backupStore.Jobs[i].Source = source
			}
			if dest != "" {
				backupStore.Jobs[i].Destination = dest
			}
			if schedule != "" {
				backupStore.Jobs[i].Schedule = schedule
			}
			backupStore.Jobs[i].Exclude = exclude
			backupStore.Jobs[i].Compress = compress
			found = true
			break
		}
	}
	backupMu.Unlock()

	if !found {
		respondJSON(w, false, "Job not found")
		return
	}
	if err := saveBackupStore(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	auditFromRequest(r, "Backup job edited", name, "system")
	respondJSON(w, true, "Job updated")
}

func handleBackupDelete(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	backupMu.Lock()
	found := false
	for i, job := range backupStore.Jobs {
		if job.ID == id {
			backupStore.Jobs = append(backupStore.Jobs[:i], backupStore.Jobs[i+1:]...)
			found = true
			break
		}
	}
	backupMu.Unlock()

	if !found {
		respondJSON(w, false, "Job not found")
		return
	}
	if err := saveBackupStore(); err != nil {
		respondJSON(w, false, "Failed to save: "+err.Error())
		return
	}
	auditFromRequest(r, "Backup job deleted", id, "system")
	respondJSON(w, true, "Job deleted")
}

func handleBackupToggle(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	backupMu.Lock()
	for i := range backupStore.Jobs {
		if backupStore.Jobs[i].ID == id {
			backupStore.Jobs[i].Enabled = !backupStore.Jobs[i].Enabled
			break
		}
	}
	backupMu.Unlock()
	saveBackupStore()
	respondJSON(w, true, "Job toggled")
}

func handleBackupRunNow(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")

	backupMu.RLock()
	var job *BackupJob
	for i := range backupStore.Jobs {
		if backupStore.Jobs[i].ID == id {
			j := backupStore.Jobs[i]
			job = &j
			break
		}
	}
	backupMu.RUnlock()
	isRunning := isJobRunning(id)

	if job == nil {
		respondJSON(w, false, "Job not found")
		return
	}
	if isRunning {
		respondJSON(w, false, "Job is already running")
		return
	}

	go executeBackup(*job)
	auditFromRequest(r, "Backup started manually", job.Name, "system")
	respondJSON(w, true, "Backup started")
}

func handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	runID := r.FormValue("run_id")

	backupMu.RLock()
	var run *BackupRun
	for i := range backupStore.History {
		if backupStore.History[i].ID == runID {
			r := backupStore.History[i]
			run = &r
			break
		}
	}
	backupMu.RUnlock()

	if run == nil {
		respondJSON(w, false, "Backup run not found")
		return
	}

	output, err := restoreBackup(*run)
	if err != nil {
		respondJSON(w, false, "Restore failed: "+err.Error()+"\n"+output)
		return
	}

	auditFromRequest(r, "Backup restored", run.JobName+" -> "+run.Source, "system")
	respondJSON(w, true, "Restore completed successfully")
}

func handleBackupDeleteRun(w http.ResponseWriter, r *http.Request) {
	runID := r.FormValue("run_id")
	deleteFile := r.FormValue("delete_file") == "true"

	backupMu.Lock()
	found := false
	for i, run := range backupStore.History {
		if run.ID == runID {
			if deleteFile && run.FilePath != "" {
				os.Remove(run.FilePath)
				if run.ManifestPath != "" {
					os.Remove(run.ManifestPath)
				}
				if run.FilePath != "" {
					os.Remove(run.FilePath + ".sha256")
				}
			}
			backupStore.History = append(backupStore.History[:i], backupStore.History[i+1:]...)
			found = true
			break
		}
	}
	backupMu.Unlock()

	if !found {
		respondJSON(w, false, "Run not found")
		return
	}
	saveBackupStore()
	respondJSON(w, true, "Run deleted")
}

func handleBackupClearHistory(w http.ResponseWriter, r *http.Request) {
	backupMu.Lock()
	backupStore.History = nil
	backupMu.Unlock()
	saveBackupStore()
	auditFromRequest(r, "Backup history cleared", "", "system")
	respondJSON(w, true, "History cleared")
}
