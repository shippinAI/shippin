package api

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/subs"
)

func TestSubscriptionReportInvalidatesMissingProviders(t *testing.T) {
	a, _, tn, _ := subsAPI(t)
	ctx := context.Background()
	subject := "report-owner-" + uuid.NewString()
	acct, err := a.db.UpsertAccount(ctx, subject, "")
	if err != nil {
		t.Fatal(err)
	}
	h, err := a.subs.EnsureHome(ctx, tn.ID, acct, "", a.terminalHost(tn.ID, acct))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"claude", "codex"} {
		if err := a.subs.SetStatus(ctx, h, id, subs.Connected, "", true); err != nil {
			t.Fatal(err)
		}
	}
	var report hostclient.AIStatus
	if err := json.Unmarshal([]byte(`{"providers":[{"provider":"claude","installed":true,"signed_in":true,"auth_known":false}]}`), &report); err != nil {
		t.Fatal(err)
	}
	if err := a.applyStatus(ctx, h, &report); err != nil {
		t.Fatal(err)
	}
	cs, err := a.subs.Connections(ctx, acct)
	if err != nil || len(cs) != 2 {
		t.Fatalf("%v %v", cs, err)
	}
	for _, c := range cs {
		if c.Status != subs.Limited {
			t.Fatalf("stale connected: %+v", c)
		}
	}
}

func TestSubscriptionCheckRejectsLegacyHostdReport(t *testing.T) {
	a, _, tn, _ := subsAPI(t)
	ctx := context.Background()
	subject := "legacy-owner-" + uuid.NewString()
	acct, _ := a.db.UpsertAccount(ctx, subject, "")
	h, _ := a.subs.EnsureHome(ctx, tn.ID, acct, "", a.terminalHost(tn.ID, acct))
	if err := a.subs.SetStatus(ctx, h, "codex", subs.Connected, "", true); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":1,"providers":[{"provider":"codex","signed_in":true}]}`))
	}))
	defer server.Close()
	a.host = hostclient.New(server.URL)
	me := &caller{id: &auth.Identity{Sub: subject}, tenants: []tenantView{{Tenant: tn, Roles: []string{"org_member"}}}}
	if w := subsCall(a, me, a.checkSubscription, "POST", tn.ID, "codex"); w.Code != 409 {
		t.Fatalf("legacy hostd accepted: %d", w.Code)
	}
	cs, _ := a.subs.Connections(ctx, acct)
	for _, c := range cs {
		if c.Status == subs.Connected {
			t.Fatal("old connected survives refresh failure")
		}
	}
}

func TestValidAIReport(t *testing.T) {
	for _, st := range []*hostclient.AIStatus{nil, {}, {Version: 1, CheckedAt: time.Now()}, {Version: 2, CheckedAt: time.Now().Add(-time.Hour)}, {Version: 2, CheckedAt: time.Now().Add(time.Hour)}} {
		if validAIReport(st) {
			t.Fatal("invalid report accepted")
		}
	}
	if !validAIReport(&hostclient.AIStatus{Version: 2, CheckedAt: time.Now()}) {
		t.Fatal("fresh empty report should invalidate all providers")
	}
}

func TestSubscriptionCancellationDoesNotSkipInvalidation(t *testing.T) {
	a, _, tn, _ := subsAPI(t)
	ctx := context.Background()
	subject := "cancel-owner-" + uuid.NewString()
	acct, _ := a.db.UpsertAccount(ctx, subject, "")
	h, _ := a.subs.EnsureHome(ctx, tn.ID, acct, "", a.terminalHost(tn.ID, acct))
	if err := a.subs.SetStatus(ctx, h, "codex", subs.Connected, "", true); err != nil {
		t.Fatal(err)
	}
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cancel(); <-r.Context().Done() }))
	defer server.Close()
	a.host = hostclient.New(server.URL)
	me := &caller{id: &auth.Identity{Sub: subject}, tenants: []tenantView{{Tenant: tn, Roles: []string{"org_member"}}}}
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) { return me, true }
	r := httptest.NewRequest("POST", "/x", nil).WithContext(requestCtx)
	r.SetPathValue("tenantId", tn.ID)
	r.SetPathValue("provider", "codex")
	w := httptest.NewRecorder()
	a.checkSubscription(w, r)
	if w.Code != 409 {
		t.Fatalf("cancelled refresh status %d", w.Code)
	}
	cs, _ := a.subs.Connections(ctx, acct)
	for _, c := range cs {
		if c.Status == subs.Connected {
			t.Fatal("cancelled host request retained connected")
		}
	}
}

func TestSubscriptionValidNegativeReportSurvivesCancelledHostContext(t *testing.T) {
	a, _, tn, _ := subsAPI(t)
	ctx := context.Background()
	acct, _ := a.db.UpsertAccount(ctx, "negative-"+uuid.NewString(), "")
	h, _ := a.subs.EnsureHome(ctx, tn.ID, acct, "", a.terminalHost(tn.ID, acct))
	if err := a.subs.SetStatus(ctx, h, "codex", subs.Connected, "", true); err != nil {
		t.Fatal(err)
	}
	var st hostclient.AIStatus
	if err := json.Unmarshal([]byte(`{"providers":[{"provider":"codex","installed":true,"signed_in":false,"auth_known":true}]}`), &st); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := a.applyFreshStatus(cancelled, h, &st); err != nil {
		t.Fatal(err)
	}
	cs, _ := a.subs.Connections(ctx, acct)
	for _, c := range cs {
		if c.ProviderID == "codex" && c.Status != subs.NeedsReauth {
			t.Fatalf("stale connected after valid negative report: %+v", c)
		}
	}
}
