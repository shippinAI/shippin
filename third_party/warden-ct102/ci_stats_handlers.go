package main

// GitHub webhook receiver — the stats fast path.
// Spec: docs/specs/runner-fleet-health-and-stats.md §3 (P2).
//
// Unauthenticated route by design: GitHub is the caller, and the HMAC
// signature (X-Hub-Signature-256 against WARDEN_GITHUB_WEBHOOK_SECRET)
// is the authentication. Without the secret configured the endpoint
// refuses all deliveries rather than accepting unsigned ones.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func verifyGitHubSignature(secret string, body []byte, header string) bool {
	if secret == "" || !strings.HasPrefix(header, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(header))
}

func handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secret := strings.TrimSpace(os.Getenv("WARDEN_GITHUB_WEBHOOK_SECRET"))
	if secret == "" {
		http.Error(w, "Webhook secret not configured", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Body read failed", http.StatusBadRequest)
		return
	}
	if !verifyGitHubSignature(secret, body, r.Header.Get("X-Hub-Signature-256")) {
		log.Printf("Webhook signature rejected from %q", r.RemoteAddr)
		http.Error(w, "Bad signature", http.StatusUnauthorized)
		return
	}
	event := r.Header.Get("X-GitHub-Event")
	switch event {
	case "ping":
		w.WriteHeader(http.StatusOK)
		return
	case "workflow_job":
		job, ok, err := parseWorkflowJobEvent(body)
		if err != nil {
			log.Printf("Webhook workflow_job parse failed: %v", err)
			http.Error(w, "Bad payload", http.StatusBadRequest)
			return
		}
		if !ok { // queued/in_progress — not a wide event yet
			w.WriteHeader(http.StatusAccepted)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := ciJobUpsert(ctx, job); err != nil {
			log.Printf("Ci job upsert failed for job %d: %v", job.ID, err)
			http.Error(w, "Store failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	default:
		w.WriteHeader(http.StatusAccepted) // acknowledged, not stored
	}
}
