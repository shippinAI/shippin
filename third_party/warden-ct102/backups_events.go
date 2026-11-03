package main

import (
	"fmt"
	"log"
	"time"
)

var (
	lastS3PingOK  bool
	lastS3PingMsg string
)

func appendBackupEvent(level, action, message string) {
	ev := BackupEvent{
		Time:    time.Now().UTC().Format("2006-01-02 15:04:05"),
		Level:   level,
		Action:  action,
		Message: message,
	}
	backupMu.Lock()
	backupStore.Events = append([]BackupEvent{ev}, backupStore.Events...)
	if len(backupStore.Events) > maxBackupEvents {
		backupStore.Events = backupStore.Events[:maxBackupEvents]
	}
	backupMu.Unlock()
	if err := saveBackupStore(); err != nil {
		log.Printf("Failed to persist backup event")
	}
	log.Printf("backup %s %s: %s", level, action, message)
}

func recentBackupEvents(n int) []BackupEvent {
	backupMu.RLock()
	defer backupMu.RUnlock()
	if n <= 0 || n > len(backupStore.Events) {
		n = len(backupStore.Events)
	}
	out := make([]BackupEvent, n)
	copy(out, backupStore.Events[:n])
	return out
}

func buildBackupChecks(pf *EstatePreflight) {
	dest := pf.Destination
	keys := dest.HasAccessKey && dest.HasSecretKey
	bucketOK := dest.Kind != "s3" || dest.Bucket != ""
	epOK := dest.Kind != "s3" || dest.Endpoint != ""

	status := "pass"
	detail := dest.Name
	fix := ""
	if dest.Kind == "s3" {
		detail = dest.Endpoint + " / " + dest.Bucket
		if !epOK || !bucketOK {
			status = "fail"
			detail = "Endpoint or bucket is empty"
			fix = "Fill endpoint and bucket in step 1, then Save."
		} else if !keys {
			status = "fail"
			detail = "Access key or secret key is not saved yet"
			fix = "Paste both keys and click Save. Blank fields keep a previously saved key."
		} else {
			status = "pass"
			detail = dest.Bucket + " on " + dest.Endpoint + " (keys saved)"
		}
	}
	pf.Checks = append(pf.Checks, BackupCheck{
		ID: "dest", Title: "1. Where copies go", Detail: detail, Fix: fix, Status: status,
	})

	pingStatus := "warn"
	pingDetail := "Not tested this session. Click Test destination."
	if lastS3PingMsg != "" {
		if lastS3PingOK {
			pingStatus = "pass"
		} else {
			pingStatus = "fail"
		}
		pingDetail = lastS3PingMsg
	} else if status != "pass" {
		pingStatus = "fail"
		pingDetail = "Save destination first."
	}
	pf.Checks = append(pf.Checks, BackupCheck{
		ID: "s3ping", Title: "2. Destination answers", Detail: pingDetail,
		Fix: "Click Test destination. A fail here means the endpoint, bucket, or keys are wrong.",
		Status: pingStatus,
	})

	stor := pf.Storage
	storStatus := "fail"
	storFix := "Click Test Warden Storage. This panel cannot mount the 400G share; a good test means Copy can stream it."
	switch {
	case stor.CopyReady && stor.SMBReady:
		storStatus = "pass"
		storFix = ""
	case stor.LocalMounted:
		storStatus = "pass"
		storFix = ""
	case stor.Message != "":
		storStatus = "fail"
	}
	pf.Checks = append(pf.Checks, BackupCheck{
		ID: "storage", Title: "2b. Warden Storage (the main backup)", Detail: stor.Message, Fix: storFix, Status: storStatus,
	})

	enabled := 0
	present := 0
	for _, t := range pf.Targets {
		if t.Enabled {
			enabled++
			if t.Present {
				present++
			}
		}
	}
	srcStatus := "pass"
	srcDetail := ""
	srcFix := ""
	switch {
	case enabled == 0:
		srcStatus = "fail"
		srcDetail = "No folders are enabled."
		srcFix = "Add a folder in step 3, or enable an existing one."
	case present == 0:
		srcStatus = "fail"
		srcDetail = "None of the enabled folders exist on this machine."
		srcFix = "Change the path, mount storage, or add a folder that is actually here (for example /etc/warden)."
	case present < enabled:
		srcStatus = "warn"
		srcDetail = fmt.Sprintf("%d of %d enabled folders exist here; missing ones will be skipped", present, enabled)
		srcFix = "Edit or disable the missing folders, or mount them."
	default:
		srcStatus = "pass"
		srcDetail = fmt.Sprintf("%d folder(s) ready to copy", present)
	}
	pf.Checks = append(pf.Checks, BackupCheck{
		ID: "sources", Title: "3. What to copy", Detail: srcDetail, Fix: srcFix, Status: srcStatus,
	})

	cov := pf.CoverageSummary
	covStatus := "pass"
	covDetail := cov.Headline
	covFix := ""
	switch {
	case cov.Required == 0:
		covStatus = "warn"
		covDetail = "Operator list is empty."
		covFix = "Reload this page. The list of what a full restore needs should appear at the top."
	case cov.RequiredGaps > 0:
		covStatus = "warn"
		covFix = "Click Add all required items, then Mount shared storage if the shared disk still shows No."
	}
	pf.Checks = append(pf.Checks, BackupCheck{
		ID: "coverage", Title: "3b. Full restore coverage", Detail: covDetail, Fix: covFix, Status: covStatus,
	})

	runStatus := "pass"
	runDetail := "Idle — ready when the checks above pass."
	if pf.Running {
		runStatus = "warn"
		runDetail = "A backup is running now. Wait for it to finish."
	}
	pf.Checks = append(pf.Checks, BackupCheck{
		ID: "run", Title: "4. Run", Detail: runDetail, Status: runStatus,
	})

	pf.S3Ping = pingDetail
}
