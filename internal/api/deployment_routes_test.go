package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/deployments"
)

func TestDeploymentRoutesRegistered(t *testing.T) {
	rec := &recorder{}
	(&API{cfg: testCfg()}).deploymentRoutes(rec)
	want := []string{
		"GET /v1/tenants/{tenantId}/deployable-services",
		"GET /v1/tenants/{tenantId}/deployment-targets",
		"GET /v1/tenants/{tenantId}/deployments",
		"POST /v1/tenants/{tenantId}/deployments",
		"GET /v1/tenants/{tenantId}/deployments/{deploymentId}",
		"PATCH /v1/tenants/{tenantId}/deployments/{deploymentId}",
		"DELETE /v1/tenants/{tenantId}/deployments/{deploymentId}",
		"GET /v1/tenants/{tenantId}/deployments/{deploymentId}/events",
		"GET /v1/tenants/{tenantId}/deployments/{deploymentId}/review",
		"POST /v1/tenants/{tenantId}/deployments/{deploymentId}/review",
	}
	if len(rec.patterns) != len(want) {
		t.Fatalf("registered routes %v", rec.patterns)
	}
	for i := range want {
		if rec.patterns[i] != want[i] {
			t.Errorf("route %d = %q, want %q", i, rec.patterns[i], want[i])
		}
	}
}

func TestConfiguredProviderLabelUsesInstallationUI(t *testing.T) {
	ui := config.UI{Providers: []config.UIProvider{{Kind: "proxmox", Label: "Shippin Prox"}}}
	if got := configuredProviderLabel(ui, deployments.ProviderProxmox); got != "Shippin Prox" {
		t.Fatalf("configured provider label = %q", got)
	}
	if got := configuredProviderLabel(config.UI{}, deployments.ProviderIncus); got != "incus" {
		t.Fatalf("unconfigured provider label fallback = %q", got)
	}
}

func TestDeploymentMutationRequiresTenantManagerAndIdempotencyKey(t *testing.T) {
	tenantID := uuid.NewString()
	makeAPI := func(role string) *API {
		return &API{cfg: testCfg(), resolveCaller: func(http.ResponseWriter, *http.Request) (*caller, bool) {
			return &caller{id: &auth.Identity{Sub: "user-1"}, managers: testCfg().Roles.Managers,
				tenants: []tenantView{{Tenant: db.Tenant{ID: tenantID}, Roles: []string{role}}}}, true
		}}
	}
	request := func() *http.Request {
		r := httptest.NewRequest("POST", "/v1/tenants/"+tenantID+"/deployments", strings.NewReader(`{"service_id":"postgres"}`))
		r.SetPathValue("tenantId", tenantID)
		return r
	}
	denied := httptest.NewRecorder()
	makeAPI("org_member").createDeployment(denied, request())
	if denied.Code != http.StatusForbidden {
		t.Fatalf("member mutation: %d %s", denied.Code, denied.Body)
	}
	missingKey := httptest.NewRecorder()
	makeAPI("org_owner").createDeployment(missingKey, request())
	if missingKey.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency key: %d %s", missingKey.Code, missingKey.Body)
	}
}

func TestDeploymentIfMatchAndErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
		code  int
	}{{`"3"`, 3, 0}, {"3", 0, http.StatusPreconditionRequired}, {`W/"3"`, 0, http.StatusPreconditionRequired}, {`"*"`, 0, http.StatusBadRequest}} {
		r := httptest.NewRequest("PATCH", "/", nil)
		r.Header.Set("If-Match", tc.value)
		w := httptest.NewRecorder()
		got, ok := deploymentIfMatch(w, r)
		if tc.code == 0 {
			if !ok || got != tc.want {
				t.Errorf("If-Match %q = %d,%t", tc.value, got, ok)
			}
		} else if ok || w.Code != tc.code {
			t.Errorf("If-Match %q response = %d,%t, want %d,false", tc.value, w.Code, ok, tc.code)
		}
	}
	w := httptest.NewRecorder()
	writeDeploymentError(w, errors.New("database detail"))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unclassified error returned %d, want generic 503", w.Code)
	}
	w = httptest.NewRecorder()
	writeDeploymentError(w, deployments.ErrInvalidConfig)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("configuration validation returned %d, want 422", w.Code)
	}
}
