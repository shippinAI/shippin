package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
)

// replayedHeaders are the response headers stored with an Idempotency-Key and replayed with it.
var replayedHeaders = []string{"Content-Type", "Location"}

// idempotent runs h once per Idempotency-Key within scope (tenant + caller) and replays the first
// response for a repeat. The same key with a different method, path or body gets 422; a repeat
// while the first is still running gets 409. 5xx responses aren't stored, so they can be retried.
// No header: h runs as before.
func (a *API) idempotent(w http.ResponseWriter, r *http.Request, scope string, h http.HandlerFunc) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		h(w, r)
		return
	}
	if _, err := uuid.Parse(key); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Idempotency-Key must be a UUID.")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		auth.Problem(w, http.StatusRequestEntityTooLarge, "Request body too large.")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	sum := sha256.Sum256(append([]byte(r.Method+" "+r.URL.Path+"\n"), body...))
	hash := hex.EncodeToString(sum[:])
	ttl := config.Sec(a.cfg.Jobs.IdempotencyHours * 3600)
	rec, err := a.db.ClaimIdemKey(r.Context(), scope, key, hash, ttl)
	switch {
	case err != nil:
		log.Printf("idempotency: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "Try again shortly.")
	case rec == nil:
		a.recordOnce(w, r, scope, key, h)
	case rec.Hash != hash:
		auth.Problem(w, http.StatusUnprocessableEntity, "This Idempotency-Key was already used for a different request.")
	case rec.Status == 0:
		auth.Problem(w, http.StatusConflict, "A request with this Idempotency-Key is still in progress.")
	default:
		for k, v := range rec.Headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Idempotent-Replayed", "true")
		w.WriteHeader(rec.Status)
		_, _ = w.Write(rec.Body)
	}
}

// recordOnce runs h, copying its response to the client and into the key's record.
func (a *API) recordOnce(w http.ResponseWriter, r *http.Request, scope, key string, h http.HandlerFunc) {
	rec := &teeWriter{ResponseWriter: w, status: http.StatusOK}
	ctx := context.WithoutCancel(r.Context()) // a client hang-up must not leave the key stuck in flight
	saved := false
	defer func() {
		if !saved { // 5xx or a panic: free the key for a retry
			_ = a.db.DropIdemKey(ctx, scope, key)
		}
	}()
	h(rec, r)
	if rec.status >= 500 {
		return
	}
	headers := map[string]string{}
	for _, k := range replayedHeaders {
		if v := w.Header().Get(k); v != "" {
			headers[k] = v
		}
	}
	if err := a.db.SaveIdemKey(ctx, scope, key, rec.status, rec.body.Bytes(), headers); err != nil {
		log.Printf("idempotency save: %v", err)
		return
	}
	saved = true
}

// teeWriter passes a response through while keeping its status and body.
type teeWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
	wrote  bool
}

func (t *teeWriter) WriteHeader(code int) {
	if !t.wrote {
		t.status, t.wrote = code, true
	}
	t.ResponseWriter.WriteHeader(code)
}

func (t *teeWriter) Write(b []byte) (int, error) {
	t.wrote = true
	t.body.Write(b)
	return t.ResponseWriter.Write(b)
}
