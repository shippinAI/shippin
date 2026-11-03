package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/agentruns"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/subs"
)

// TestPromptProjectFlow: M4 (implementation-plan-3) — prompt cap, idempotent-safe create, waiting_ai
// until the owner's own sign-in is connected, then the prompt goes to hostd in a request body only
// (never a URL), is dropped from the console DB, and the agent starts.
func TestPromptProjectFlow(t *testing.T) {
	a, d, tn, _ := subsAPI(t)
	ctx := context.Background()
	a.runs = &agentruns.Store{Pool: d.Pool}
	a.jobs = jobs.NewStore(d.Pool)
	a.runner = jobs.NewRunner(a.jobs, jobs.Settings{})
	a.cfg.Jobs.MaxAttempts = 3
	a.cfg.Prompt = config.Prompt{Enabled: true, MaxBytes: 8192, Agents: []string{"claude", "codex"}, DefaultTemplate: "blank",
		TermPath: "/_term/"}
	_, _ = d.Pool.Exec(ctx, `INSERT INTO templates(id,slug,name,image) VALUES($1,'blank','Blank','tpl') ON CONFLICT (slug) DO NOTHING`, uuid.NewString())
	var mu sync.Mutex
	var urls, bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		urls, bodies = append(urls, r.Method+" "+r.URL.String()), append(bodies, string(b))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"started":true}`))
	}))
	t.Cleanup(srv.Close)
	a.host = hostclient.New(srv.URL)
	me := &caller{id: &auth.Identity{Sub: "sub-" + uuid.NewString(), Email: "me@acme.test"},
		tenants: []tenantView{{Tenant: tn, Roles: []string{"org_owner"}}}}
	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/x", strings.NewReader(body))
		w := httptest.NewRecorder()
		a.createFromPrompt(w, r, me, &me.tenants[0])
		return w
	}
	if w := post(`{"prompt":"` + strings.Repeat("a", 8193) + `","provider":"claude","stack":"blank"}`); w.Code != 400 {
		t.Fatalf("8 KiB cap: %d", w.Code)
	}
	if w := post(`{"prompt":"x","provider":"gemini","stack":"blank"}`); w.Code != 400 {
		t.Fatalf("unknown agent: %d", w.Code)
	}
	w := post(`{"prompt":"make a todo app\"; rm -rf ~ $(id) ` + "`whoami`" + `","provider":"claude","stack":"blank"}`)
	if w.Code != 202 || strings.Contains(w.Body.String(), "rm -rf") && !strings.Contains(w.Body.String(), "prompt_preview") {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var pid string
	_ = d.Pool.QueryRow(ctx, `SELECT project_id FROM agent_runs WHERE created_by=$1`, me.id.Sub).Scan(&pid)
	run, err := a.runs.Latest(ctx, tn.ID, pid)
	if err != nil || run.State != agentruns.Queued {
		t.Fatalf("run: %+v %v", run, err)
	}
	ws := uuid.NewString()
	if out, err := a.deliverAndStart(ctx, run, ws, false, func(string) {}); err != nil || len(urls) != 0 {
		t.Fatalf("not signed in must not touch the devspace: %v %v %v", out, err, urls)
	}
	if run, _ = a.runs.Get(ctx, tn.ID, run.ID); run.State != agentruns.WaitingAI {
		t.Fatalf("want waiting_ai, got %s", run.State)
	}
	h, err := a.subs.EnsureHome(ctx, tn.ID, run.AccountID, "", subs.UtilityLabel(run.AccountID)+".ws.test")
	if err != nil {
		t.Fatal(err)
	}
	_ = a.subs.SetHomeState(ctx, h.ID, "ready", "")
	_ = a.subs.SetStatus(ctx, h, "claude", subs.Connected, "", true)
	if _, err := a.deliverAndStart(ctx, run, ws, false, func(string) {}); err == nil || !strings.Contains(err.Error(), "Devagotchi") {
		t.Fatalf("connected login must not bypass missing runner grant: %v", err)
	}
	if len(urls) != 0 {
		t.Fatalf("unsafe workspace execution: %v", urls)
	}
	if p, _ := a.runs.Pending(ctx, run.ID); p == "" {
		t.Fatal("undelivered prompt must be preserved")
	}
	if run, _ = a.runs.Get(ctx, tn.ID, run.ID); run.State != agentruns.Failed {
		t.Fatalf("want explicit unavailable error, got %s", run.State)
	}
}
