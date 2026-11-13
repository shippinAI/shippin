package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/runfleet"
)

func newHandler(svc *runfleet.Service, cfg serverConfig) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /v1/github/webhook", func(w http.ResponseWriter, r *http.Request) {
		if cfg.WebhookSecret == "" {
			problem(w, http.StatusServiceUnavailable, "webhook secret is not configured")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if err != nil {
			problem(w, http.StatusBadRequest, "unreadable body")
			return
		}
		if !runfleet.VerifyWebhook(cfg.WebhookSecret, r.Header.Get("X-Hub-Signature-256"), body) {
			problem(w, http.StatusUnauthorized, "webhook signature was rejected")
			return
		}
		if err := svc.HandleWebhook(r.Header.Get("X-GitHub-Event"), body); err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	protected := http.NewServeMux()
	protected.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if err != nil {
			problem(w, http.StatusBadRequest, "unreadable body")
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			problem(w, http.StatusBadRequest, "request is not JSON")
			return
		}
		result, rpcErr := dispatch(r.Context(), svc, req.Method, req.Params)
		msg := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if rpcErr != nil {
			msg["error"] = map[string]any{"code": rpcErr.code, "message": rpcErr.message}
		} else {
			msg["result"] = result
		}
		writeJSON(w, http.StatusOK, msg)
	})
	protected.HandleFunc("GET /v1/sizes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": svc.Sizes()})
	})
	protected.HandleFunc("GET /v1/runners", func(w http.ResponseWriter, r *http.Request) {
		items, err := svc.ListRunners()
		if err != nil {
			problem(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	protected.HandleFunc("GET /v1/runners/{id}/health", func(w http.ResponseWriter, r *http.Request) {
		item, err := svc.RunnerHealth(r.Context(), r.PathValue("id"))
		if err != nil {
			problem(w, http.StatusNotFound, "runner not found")
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
	protected.HandleFunc("POST /v1/connect", func(w http.ResponseWriter, r *http.Request) {
		var in runfleet.ConnectInput
		if !readJSON(w, r, &in) {
			return
		}
		item, err := svc.Connect(r.Context(), in)
		if err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
	protected.HandleFunc("POST /v1/approve", func(w http.ResponseWriter, r *http.Request) {
		var in runfleet.ApproveInput
		if !readJSON(w, r, &in) {
			return
		}
		item, err := svc.Approve(r.Context(), in)
		if err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
	protected.HandleFunc("GET /v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		items, err := svc.ListJobs(r.URL.Query().Get("repo"))
		if err != nil {
			problem(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	protected.HandleFunc("POST /v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		var in runfleet.IngestInput
		if !readJSON(w, r, &in) {
			return
		}
		item, err := svc.IngestJob(in)
		if err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
	protected.HandleFunc("GET /v1/jobs/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		item, err := svc.JobLogs(r.PathValue("id"))
		if err != nil {
			problem(w, http.StatusNotFound, "job not found")
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
	protected.HandleFunc("POST /v1/jobs/{id}/explain", func(w http.ResponseWriter, r *http.Request) {
		item, err := svc.Explain(r.PathValue("id"))
		if err != nil {
			problem(w, http.StatusNotFound, "job not found")
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
	protected.HandleFunc("POST /v1/runs-on", func(w http.ResponseWriter, r *http.Request) {
		var in runfleet.RunsOnInput
		if !readJSON(w, r, &in) {
			return
		}
		item, err := svc.SetRunsOn(r.Context(), in)
		if err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
	protected.HandleFunc("GET /v1/usage", func(w http.ResponseWriter, r *http.Request) {
		item, err := svc.Usage(r.URL.Query().Get("account"), r.URL.Query().Get("period"))
		if err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, item)
	})
	protected.HandleFunc("PUT /v1/retention", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Repo string `json:"repo"`
			Days int    `json:"days"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		if err := svc.SetRetention(in.Repo, in.Days); err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, in)
	})
	mux.Handle("/", requireToken(cfg.Token, protected))
	return mux
}

func requireToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || !hmacEqual(got, token) {
			problem(w, http.StatusUnauthorized, "bearer token was rejected")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hmacEqual(a, b string) bool {
	if b == "" {
		return false
	}
	return runfleetEqual(a, b)
}

func readJSON(w http.ResponseWriter, r *http.Request, dest any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		problem(w, http.StatusBadRequest, "request JSON was rejected")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func problem(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"message": message, "status": status})
}
