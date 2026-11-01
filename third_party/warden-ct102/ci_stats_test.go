package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyGitHubSignature(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"zen":"Keep it logically awesome."}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	good := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyGitHubSignature(secret, body, good) {
		t.Error("valid signature rejected")
	}
	if verifyGitHubSignature(secret, body, "sha256=deadbeef") {
		t.Error("bad signature accepted")
	}
	if verifyGitHubSignature(secret, body, "") {
		t.Error("empty header accepted")
	}
	if verifyGitHubSignature("", body, good) {
		t.Error("empty secret accepted")
	}
}

func TestParseWorkflowJobEvent(t *testing.T) {
	completed := []byte(`{
		"action": "completed",
		"workflow_job": {
			"id": 42, "run_id": 7, "name": "nightly-windows",
			"workflow_name": "nightly", "conclusion": "success",
			"created_at": "2026-07-28T07:00:00Z",
			"started_at": "2026-07-28T07:01:00Z",
			"completed_at": "2026-07-28T07:09:00Z",
			"runner_name": "devpulse-runner-win-01",
			"head_sha": "abc123", "head_branch": "main",
			"html_url": "https://github.com/x/y/runs/42"
		},
		"repository": {"full_name": "example-org/devpulselabs"}
	}`)
	job, ok, err := parseWorkflowJobEvent(completed)
	if err != nil || !ok {
		t.Fatalf("completed event not parsed: ok=%v err=%v", ok, err)
	}
	if job.ID != 42 || job.Repo != "example-org/devpulselabs" ||
		job.Conclusion != "success" || job.RunnerName != "devpulse-runner-win-01" {
		t.Errorf("unexpected job fields: %+v", job)
	}
	if job.QueuedAt == nil || job.CompletedAt.Sub(job.StartedAt).Minutes() != 8 {
		t.Errorf("timestamps wrong: queued=%v exec=%v", job.QueuedAt, job.CompletedAt.Sub(job.StartedAt))
	}

	queued := []byte(`{"action":"queued","workflow_job":{"id":1,"started_at":"2026-07-28T07:00:00Z"},"repository":{"full_name":"x/y"}}`)
	if _, ok, _ := parseWorkflowJobEvent(queued); ok {
		t.Error("queued event should not produce a wide event")
	}
}
