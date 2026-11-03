package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func resetBackupStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	backupPath = filepath.Join(dir, "backups.json")
	infisicalDisabled = true
	resetDestSecrets()
	backupMu.Lock()
	backupStore = BackupStore{}
	seedEstateDefaultsLocked()
	backupMu.Unlock()
	if err := saveBackupStore(); err != nil {
		t.Fatalf("save backup store: %v", err)
	}
	return dir
}

func TestSeedEstateDefaults(t *testing.T) {
	resetBackupStore(t)
	backupMu.RLock()
	defer backupMu.RUnlock()
	if backupStore.Estate.Destination != estateDefaultDest {
		t.Fatalf("destination = %q, want %q", backupStore.Estate.Destination, estateDefaultDest)
	}
	if len(backupStore.Estate.Targets) < 3 {
		t.Fatalf("targets = %d, want at least 3", len(backupStore.Estate.Targets))
	}
	if backupStore.Estate.Targets[0].ID != "warden-config" {
		t.Fatalf("first target = %q", backupStore.Estate.Targets[0].ID)
	}
	if !backupStore.Estate.Compress {
		t.Fatal("compress should default on")
	}
	if backupStore.Estate.ActiveDestination != estateDestShippinS3 {
		t.Fatalf("active dest = %q", backupStore.Estate.ActiveDestination)
	}
	if len(backupStore.Estate.Destinations) == 0 || backupStore.Estate.Destinations[0].Name != "shippinS3 storage" {
		t.Fatalf("expected shippinS3 storage destination, got %#v", backupStore.Estate.Destinations)
	}
	if backupStore.Estate.Destinations[0].Provider != "s3" {
		t.Fatalf("provider = %q", backupStore.Estate.Destinations[0].Provider)
	}
}

func TestProbeMissingTargets(t *testing.T) {
	resetBackupStore(t)
	pf := buildEstatePreflight(false)
	if pf.Ready {
		t.Fatal("preflight should not be ready when sources are missing")
	}
	if len(pf.Blocking) == 0 {
		t.Fatal("expected blocking reasons")
	}
	foundStorage, foundHome := false, false
	for _, tgt := range pf.Targets {
		if tgt.ID == estateTargetStorage && !tgt.Present {
			foundStorage = true
		}
		if tgt.ID == estateTargetHome && !tgt.Present {
			foundHome = true
		}
	}
	if !foundStorage || !foundHome {
		t.Fatalf("expected both targets missing, storage=%v home=%v", foundStorage, foundHome)
	}
}

func TestArchiveDirectoryRoundTrip(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	nested := filepath.Join(src, "projects", "shippin")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("warden-storage fixture\n")
	if err := os.WriteFile(filepath.Join(nested, "readme.txt"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, ".hidden"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

	archive, count, err := archiveDirectory(src, dst, "shippin-test", true, "test")
	if err != nil {
		t.Fatalf("archiveDirectory: %v", err)
	}
	if count < 3 {
		t.Fatalf("file count = %d, want at least 3", count)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("archive missing: %v", err)
	}

	sum, err := sha256File(archive)
	if err != nil {
		t.Fatalf("sha256: %v", err)
	}
	if len(sum) != 64 {
		t.Fatalf("sha256 length = %d", len(sum))
	}

	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	foundReadme := false
	foundHidden := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(hdr.Name, "readme.txt") {
			foundReadme = true
			got, _ := io.ReadAll(tr)
			if string(got) != string(payload) {
				t.Fatalf("readme contents = %q", got)
			}
		}
		if strings.HasSuffix(hdr.Name, ".hidden") {
			foundHidden = true
		}
	}
	if !foundReadme || !foundHidden {
		t.Fatalf("archive missing files readme=%v hidden=%v", foundReadme, foundHidden)
	}
}

func TestHandleBackupsRendersFullTab(t *testing.T) {
	resetBackupStore(t)
	req := httptest.NewRequest(http.MethodGet, "/backups", nil)
	rec := httptest.NewRecorder()
	handleBackups(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Full Backup",
		"Warden Storage",
		"Operator home",
		"/home/wardenop",
		"warden-storage",
		"Start full backup",
		"shippinS3 storage",
		"Add a folder",
		"Activity log",
		"Access key",
		"Secret key",
		"Step 1",
		"What should be copied",
		"Panel settings",
		"Warden Storage",
		"Add all required items",
		"Never copied",
		"Full restore coverage",
		"Copy Warden Storage",
		"Test Warden Storage",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q", want)
		}
	}
}

func TestHandleEstateRunBlockedWhenMissing(t *testing.T) {
	resetBackupStore(t)
	form := url.Values{}
	form.Set("action", "run_estate")
	form.Set("destination", "/var/backups/shippin")
	req := httptest.NewRequest(http.MethodPost, "/api/backups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleAPIBackups(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["success"] == true {
		t.Fatalf("expected failure, got %v", resp)
	}
	msg, _ := resp["message"].(string)
	if msg == "" {
		t.Fatal("expected a refusal message")
	}
}

func TestHandleEstateSave(t *testing.T) {
	resetBackupStore(t)
	form := url.Values{}
	form.Set("action", "save_estate")
	form.Set("dest_id", estateDestShippinS3)
	form.Set("s3_endpoint", "https://s3.example.invalid")
	form.Set("s3_bucket", "shippin")
	form.Set("s3_prefix", "shippin/estate")
	form.Set("compress", "on")
	form.Set("source_warden-storage", "/workspace/warden-storage")
	form.Set("enabled_warden-storage", "on")
	form.Set("source_wardenop-home", "/home/wardenop")
	form.Set("enabled_wardenop-home", "off")
	req := httptest.NewRequest(http.MethodPost, "/api/backups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleAPIBackups(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	backupMu.RLock()
	defer backupMu.RUnlock()
	if backupStore.Estate.ActiveDestination != estateDestShippinS3 {
		t.Fatalf("active dest = %q", backupStore.Estate.ActiveDestination)
	}
	if backupStore.Estate.Destinations[0].Bucket != "shippin" {
		t.Fatalf("bucket = %q", backupStore.Estate.Destinations[0].Bucket)
	}
	byID := map[string]EstateTarget{}
	for _, t := range backupStore.Estate.Targets {
		byID[t.ID] = t
	}
	if byID[estateTargetHome].Enabled {
		t.Fatal("operator home should be disabled")
	}
	if byID[estateTargetStorage].Source != "/workspace/warden-storage" {
		t.Fatalf("storage source = %q", byID[estateTargetStorage].Source)
	}
}

func TestHandleEditAndMoveSource(t *testing.T) {
	resetBackupStore(t)
	form := url.Values{}
	form.Set("action", "add_source")
	form.Set("name", "Config")
	form.Set("source", "/etc/warden")
	req := httptest.NewRequest(http.MethodPost, "/api/backups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleAPIBackups(rec, req)

	backupMu.RLock()
	id := backupStore.Estate.Targets[len(backupStore.Estate.Targets)-1].ID
	backupMu.RUnlock()

	edit := url.Values{}
	edit.Set("action", "edit_source")
	edit.Set("id", id)
	edit.Set("name", "Warden config")
	edit.Set("source", "/etc/warden")
	req = httptest.NewRequest(http.MethodPost, "/api/backups", strings.NewReader(edit.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handleAPIBackups(rec, req)

	move := url.Values{}
	move.Set("action", "move_source")
	move.Set("id", id)
	move.Set("dir", "up")
	req = httptest.NewRequest(http.MethodPost, "/api/backups", strings.NewReader(move.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handleAPIBackups(rec, req)

	backupMu.RLock()
	defer backupMu.RUnlock()
	found := false
	for _, t := range backupStore.Estate.Targets {
		if t.ID == id && t.Name == "Warden config" {
			found = true
		}
	}
	if !found {
		t.Fatal("edited source not found")
	}
	if len(backupStore.Events) == 0 {
		t.Fatal("expected activity events")
	}
}

func TestHandleAddAndDeleteSource(t *testing.T) {
	resetBackupStore(t)
	form := url.Values{}
	form.Set("action", "add_source")
	form.Set("name", "Capsule workspace")
	form.Set("source", "/workspace/WardenClyffe-latest")
	req := httptest.NewRequest(http.MethodPost, "/api/backups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleAPIBackups(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("add status = %d body=%s", rec.Code, rec.Body.String())
	}
	backupMu.RLock()
	if len(backupStore.Estate.Targets) != 4 {
		backupMu.RUnlock()
		t.Fatalf("targets = %d, want 4", len(backupStore.Estate.Targets))
	}
	id := backupStore.Estate.Targets[len(backupStore.Estate.Targets)-1].ID
	backupMu.RUnlock()

	del := url.Values{}
	del.Set("action", "delete_source")
	del.Set("id", id)
	req = httptest.NewRequest(http.MethodPost, "/api/backups", strings.NewReader(del.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	handleAPIBackups(rec, req)
	backupMu.RLock()
	defer backupMu.RUnlock()
	if len(backupStore.Estate.Targets) != 3 {
		t.Fatalf("targets after delete = %d", len(backupStore.Estate.Targets))
	}
}

func TestUIKeysMakeS3Ready(t *testing.T) {
	resetBackupStore(t)
	form := url.Values{}
	form.Set("action", "save_estate")
	form.Set("dest_id", estateDestShippinS3)
	form.Set("s3_endpoint", "https://s3.example.invalid")
	form.Set("s3_bucket", "shippin")
	form.Set("s3_access_key", "AKIAUI")
	form.Set("s3_secret_key", "secret-ui-value")
	req := httptest.NewRequest(http.MethodPost, "/api/backups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleAPIBackups(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if destAccessKey(estateDestShippinS3) != "AKIAUI" || destSecretKey(estateDestShippinS3) != "secret-ui-value" {
		t.Fatal("UI keys were not saved")
	}
	pf := buildEstatePreflight(false)
	if !pf.Destination.CredsReady {
		t.Fatalf("creds should be ready from UI keys: %+v", pf.Destination)
	}
	if pf.Destination.CredsSource != "ui" {
		t.Fatalf("source = %q", pf.Destination.CredsSource)
	}
	body := rec.Body.String()
	if strings.Contains(body, "secret-ui-value") {
		t.Fatal("secret leaked in save response")
	}
}

func TestS3PreflightBlocksWithoutCreds(t *testing.T) {
	resetBackupStore(t)
	infisicalMu.Lock()
	infisicalCached = infisicalS3Status{}
	infisicalAt = time.Time{}
	infisicalMu.Unlock()
	pf := buildEstatePreflight(false)
	if pf.Destination.Kind != "s3" {
		t.Fatalf("kind = %q", pf.Destination.Kind)
	}
	if pf.Destination.CredsReady {
		t.Fatal("creds should not be ready without env")
	}
	blocked := false
	for _, b := range pf.Blocking {
		low := strings.ToLower(b)
		if strings.Contains(low, "access key") || strings.Contains(low, "secret key") || strings.Contains(low, "bucket") {
			blocked = true
		}
	}
	if !blocked {
		t.Fatalf("expected key/bucket block, got %v", pf.Blocking)
	}
}

func TestFirstExistingPath(t *testing.T) {
	dir := t.TempDir()
	if got := firstExistingPath([]string{"/no/such/path", dir}); got != dir {
		t.Fatalf("got %q want %q", got, dir)
	}
	if got := firstExistingPath([]string{"/no/such/path"}); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestBuildCoverageCatalog(t *testing.T) {
	resetBackupStore(t)
	items := buildCoverage()
	if len(items) < 15 {
		t.Fatalf("catalog size = %d, want at least 15", len(items))
	}
	foundPanel, foundDisk, foundGroup := false, false, false
	for _, it := range items {
		if it.Name == "Panel settings" && it.Required && it.Path == "/etc/warden" {
			foundPanel = true
			if it.Present {
				if !it.InList || !it.Enabled {
					t.Fatalf("panel settings should already be in the default copy list: %+v", it)
				}
			}
		}
		if it.ID == estateTargetStorage && it.Required {
			foundDisk = true
		}
		if it.ShowGroup && it.Group == "This panel" {
			foundGroup = true
		}
		if it.StatusLabel == "" {
			t.Fatalf("missing status label for %s", it.ID)
		}
	}
	if !foundPanel || !foundDisk || !foundGroup {
		t.Fatalf("catalog missing core rows panel=%v disk=%v group=%v", foundPanel, foundDisk, foundGroup)
	}
	sum := summarizeCoverage(items)
	if sum.Headline == "" || sum.Required < 5 {
		t.Fatalf("summary = %+v", sum)
	}
}

func TestHandleAddCoverage(t *testing.T) {
	resetBackupStore(t)
	form := url.Values{}
	form.Set("action", "add_coverage")
	form.Set("id", "warden-src")
	req := httptest.NewRequest(http.MethodPost, "/api/backups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleAPIBackups(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	backupMu.RLock()
	defer backupMu.RUnlock()
	found := false
	for _, tgt := range backupStore.Estate.Targets {
		if tgt.ID == "warden-src" && tgt.Enabled && tgt.Source == "/var/lib/shippin/warden-src/warden" {
			found = true
		}
	}
	if !found {
		t.Fatalf("warden-src not added: %+v", backupStore.Estate.Targets)
	}
	if len(backupStore.Events) == 0 {
		t.Fatal("expected add_coverage event")
	}
	if backupStore.Events[0].Action != "add_coverage" {
		t.Fatalf("event action = %q", backupStore.Events[0].Action)
	}
}

func TestHandleAddCoverageRequired(t *testing.T) {
	resetBackupStore(t)
	form := url.Values{}
	form.Set("action", "add_coverage_required")
	req := httptest.NewRequest(http.MethodPost, "/api/backups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleAPIBackups(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	items := buildCoverage()
	for _, it := range items {
		if it.Required && (!it.InList || !it.Enabled) {
			t.Fatalf("required item not in copy list: %+v", it)
		}
	}
	if len(backupStore.Events) == 0 {
		t.Fatal("expected activity event")
	}
}

func TestArchiveFileRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	dst := t.TempDir()
	src := filepath.Join(srcDir, "warden.service")
	if err := os.WriteFile(src, []byte("[Unit]\nDescription=Warden\n"), 0644); err != nil {
		t.Fatal(err)
	}
	archive, count, err := archiveDirectory(src, dst, "unit-test", true, "warden-unit")
	if err != nil {
		t.Fatalf("archive file: %v", err)
	}
	if count < 1 {
		t.Fatalf("file count = %d", count)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("archive missing: %v", err)
	}
}

func TestProbeAcceptsFileSource(t *testing.T) {
	resetBackupStore(t)
	f, err := os.CreateTemp("", "warden-unit-*.service")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()
	st := probeEstateTarget(EstateTarget{ID: "warden-unit", Name: "unit", Source: f.Name(), Enabled: true}, false)
	if !st.Present {
		t.Fatalf("file source should be present: %+v", st)
	}
}

func TestParseSMBBlocks(t *testing.T) {
	in := "  README.txt  N 734\n\n411725224 blocks of size 1024. 388027056 blocks available\n"
	total, avail, used, ok := parseSMBBlocks(in)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if total != 411725224*1024 || avail != 388027056*1024 {
		t.Fatalf("total=%d avail=%d", total, avail)
	}
	if used != total-avail {
		t.Fatalf("used=%d", used)
	}
}

func TestLiveSMBShareIfCredPresent(t *testing.T) {
	cred := "/root/.config/Warden/smb-warden-storage.cred"
	if _, err := os.Stat(cred); err != nil {
		t.Skip("no production SMB cred file")
	}
	prev := backupPath
	backupPath = "/root/.config/Warden/backups.json"
	t.Cleanup(func() { backupPath = prev })
	s := loadSMBShare()
	if s.missing() != "" {
		t.Fatalf("production cred incomplete: %s", s.missing())
	}
	_, _, _, listing, err := probeSMBShareUncached(s)
	if err != nil {
		t.Fatal(err)
	}
	if listing == "" {
		t.Fatal("empty SMB listing")
	}
	t.Log(listing)
}

func TestSMBShareMissingCred(t *testing.T) {
	dir := t.TempDir()
	backupPath = filepath.Join(dir, "backups.json")
	s := loadSMBShare()
	if s.missing() == "" {
		t.Fatal("expected missing username/password without cred file")
	}
	if !strings.Contains(s.missing(), "username") || !strings.Contains(s.missing(), "password") {
		t.Fatalf("missing = %q", s.missing())
	}
}

func TestSaveWritesActivityEvent(t *testing.T) {
	resetBackupStore(t)
	form := url.Values{}
	form.Set("action", "save_estate")
	form.Set("dest_id", estateDestShippinS3)
	form.Set("s3_endpoint", "https://s3.example.invalid")
	form.Set("s3_bucket", "bucket-test")
	req := httptest.NewRequest(http.MethodPost, "/api/backups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handleAPIBackups(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	backupMu.RLock()
	defer backupMu.RUnlock()
	if len(backupStore.Events) == 0 || backupStore.Events[0].Action != "save" {
		t.Fatalf("expected save event, got %+v", backupStore.Events)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatal("backups.json was not written")
	}
}
