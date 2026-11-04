package api

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIdempotencyReplaysFirstResponse(t *testing.T) {
	a, _, tn := notifyAPI(t)
	a.cfg.Jobs.IdempotencyHours = 24
	var runs atomic.Int32
	h := func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		n := runs.Add(1)
		w.Header().Set("Location", "/v1/jobs/"+string(rune('0'+n)))
		writeStatus(w, http.StatusAccepted, map[string]any{"run": n, "echo": string(b)})
	}
	call := func(key, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/tenants/x/workspaces", strings.NewReader(body))
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		w := httptest.NewRecorder()
		a.idempotent(w, r, tn.ID+":u", h)
		return w
	}
	key := uuid.NewString()
	first := call(key, `{"name":"a"}`)
	again := call(key, `{"name":"a"}`)
	if first.Code != 202 || again.Code != 202 || first.Body.String() != again.Body.String() {
		t.Fatalf("replay differs: %d %s / %d %s", first.Code, first.Body, again.Code, again.Body)
	}
	if again.Header().Get("Location") != first.Header().Get("Location") || again.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay headers: %v", again.Header())
	}
	if runs.Load() != 1 {
		t.Fatalf("handler ran %d times", runs.Load())
	}
	if w := call(key, `{"name":"b"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("different body with the same key: %d", w.Code)
	}
	if w := call("not-a-uuid", `{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad key: %d", w.Code)
	}
	call("", `{}`)
	call("", `{}`)
	if runs.Load() != 3 {
		t.Fatalf("no key must run every time, ran %d", runs.Load())
	}
}

func TestIdempotencyFreesKeyAfter5xx(t *testing.T) {
	a, _, tn := notifyAPI(t)
	a.cfg.Jobs.IdempotencyHours = 24
	fail := true
	h := func(w http.ResponseWriter, _ *http.Request) {
		if fail {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		writeStatus(w, http.StatusCreated, map[string]string{"ok": "yes"})
	}
	key := uuid.NewString()
	req := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/p", strings.NewReader("{}"))
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		a.idempotent(w, r, tn.ID, h)
		return w
	}
	if w := req(); w.Code != 503 {
		t.Fatalf("first: %d", w.Code)
	}
	fail = false
	if w := req(); w.Code != 201 {
		t.Fatalf("a 5xx must not be replayed: %d", w.Code)
	}
}

func TestIdempotencyInFlightConflicts(t *testing.T) {
	a, d, tn := notifyAPI(t)
	a.cfg.Jobs.IdempotencyHours = 24
	key := uuid.NewString()
	sum := sha256.Sum256([]byte("POST /v1/p\n{}"))
	if rec, err := d.ClaimIdemKey(t.Context(), tn.ID, key, hex.EncodeToString(sum[:]), time.Hour); err != nil || rec != nil {
		t.Fatalf("reserve: %v %+v", err, rec)
	}
	r := httptest.NewRequest("POST", "/v1/p", strings.NewReader("{}"))
	r.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	a.idempotent(w, r, tn.ID, func(http.ResponseWriter, *http.Request) { t.Fatal("ran while in flight") })
	if w.Code != http.StatusConflict {
		t.Fatalf("in flight: %d", w.Code)
	}
}

func TestIdempotencyBindsIfMatchPrecondition(t *testing.T) {
	a, _, tn := notifyAPI(t)
	a.cfg.Jobs.IdempotencyHours = 24
	key := uuid.NewString()
	call := func(etag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PATCH", "/v1/tenants/x/deployments/y", strings.NewReader(`{"desired_state":"present"}`))
		r.Header.Set("Idempotency-Key", key)
		r.Header.Set("If-Match", etag)
		w := httptest.NewRecorder()
		a.idempotent(w, r, tn.ID+":u", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, map[string]bool{"updated": true}) })
		return w
	}
	if got := call(`"1"`); got.Code != http.StatusOK {
		t.Fatalf("initial request: %d", got.Code)
	}
	if got := call(`"1"`); got.Code != http.StatusOK || got.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("same precondition replay: %d %v", got.Code, got.Header())
	}
	if got := call(`"2"`); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("changed precondition with same key: %d", got.Code)
	}
}
