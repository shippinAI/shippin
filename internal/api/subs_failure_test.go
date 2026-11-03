package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hostclient"
)

func TestSubscriptionStopFailureSurvivesCancellationAndTouch(t *testing.T) {
	a, d, tn, calls := subsAPI(t)
	ctx := context.Background()
	me := &caller{id: &auth.Identity{Sub: "cleanup-" + uuid.NewString(), Email: "me@acme.test"},
		tenants: []tenantView{{Tenant: tn, Roles: []string{"org_member"}}}}
	acct, err := d.UpsertAccount(ctx, me.id.Sub, me.id.Email)
	if err != nil {
		t.Fatal(err)
	}
	h, err := a.subs.EnsureHome(ctx, tn.ID, acct, "", a.terminalHost(tn.ID, acct))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.subs.SetUtility(ctx, h.ID, "running", true); err != nil {
		t.Fatal(err)
	}
	workingHost := a.host
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer broken.Close()
	a.host = hostclient.New(broken.URL)
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) { return me, true }
	r := httptest.NewRequest("DELETE", "/x", nil).WithContext(requestCtx)
	r.SetPathValue("tenantId", tn.ID)
	w := httptest.NewRecorder()
	a.closeTerminal(w, r)
	if w.Code != 502 {
		t.Fatalf("stop: %d", w.Code)
	}
	failed, err := a.subs.HomeByID(ctx, h.ID)
	if err != nil || failed.UtilityState != "error" || failed.State != "error" {
		t.Fatalf("lost cleanup: %+v %v", failed, err)
	}
	if w := subsCall(a, me, a.touchTerminal, "POST", tn.ID, ""); w.Code != 200 {
		t.Fatalf("touch: %d", w.Code)
	}
	// The last touch is fresh: only the error state can make this immediately eligible.
	a.host = workingHost
	a.stopIdleTerminals(ctx)
	got, err := a.subs.HomeByID(ctx, h.ID)
	if err != nil || got.UtilityState != "stopped" || got.State != "creating" || got.Error != "" ||
		!hasCall(calls(), "/utilities/"+acct+"/stop") {
		t.Fatalf("cleanup was postponed by touch: %+v %v", got, err)
	}
	if err := a.utilityFailed(requestCtx, h.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.utilityStopped(requestCtx, h.ID); err != nil {
		t.Fatal(err)
	}
	got, err = a.subs.HomeByID(ctx, h.ID)
	if err != nil || got.UtilityState != "stopped" || got.State != "creating" || got.Error != "" {
		t.Fatalf("canceled success write lost: %+v %v", got, err)
	}
}
