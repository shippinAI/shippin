package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/runfleet"
)

func TestServeAuthWebhookAndRunsOn(t *testing.T) {
	svc, err := runfleet.Open(t.TempDir(), runfleet.Options{
		Now: func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := newHandler(svc, serverConfig{Token: "test-token", WebhookSecret: "hook-secret"})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/healthz")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("health %v %v", res, err)
	}
	res.Body.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/connect", strings.NewReader(`{}`))
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth %v %v", res, err)
	}
	res.Body.Close()

	body := []byte(`{"action":"created","installation":{"id":3,"account":{"login":"octocat","type":"Organization"}}}`)
	mac := hmac.New(sha256.New, []byte("hook-secret"))
	_, _ = mac.Write(body)
	bad, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/github/webhook", bytes.NewReader(body))
	bad.Header.Set("X-GitHub-Event", "installation")
	bad.Header.Set("X-Hub-Signature-256", "sha256=dead")
	res, err = http.DefaultClient.Do(bad)
	if err != nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad sig %v %v", res, err)
	}
	res.Body.Close()

	good, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/github/webhook", bytes.NewReader(body))
	good.Header.Set("X-GitHub-Event", "installation")
	good.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	res, err = http.DefaultClient.Do(good)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("good sig %v %v", res, err)
	}
	res.Body.Close()

	payload := `{"account":"octocat","account_kind":"user","repos":["octocat/hello"],"size":"linux-x64-2","facts":[{"full_name":"octocat/hello","license":"MIT","workflows":[{"path":".github/workflows/ci.yml","content":"runs-on: ubuntu-latest\n"}]}]}`
	creq, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/connect", strings.NewReader(payload))
	creq.Header.Set("Authorization", "Bearer test-token")
	creq.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("connect status %d", res.StatusCode)
	}
	var connected map[string]any
	if err := json.NewDecoder(res.Body).Decode(&connected); err != nil {
		t.Fatal(err)
	}
	if connected["runs_on"] != "shippin-linux-x64-2" {
		t.Fatal(connected)
	}

	runs, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/runs-on", strings.NewReader(`{"repo":"octocat/hello","size":"linux-x64-2","workflows":[{"path":".github/workflows/ci.yml","content":"runs-on: ubuntu-latest\n"}]}`))
	runs.Header.Set("Authorization", "Bearer test-token")
	res, err = http.DefaultClient.Do(runs)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var plan map[string]any
	if err := json.NewDecoder(res.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	if plan["dry_run"] != true {
		t.Fatalf("apply default was not dry-run: %v", plan)
	}
}
