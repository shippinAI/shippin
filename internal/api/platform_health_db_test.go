package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
)

func TestPlatformHealthServiceLifecycleAndReadOnlyList(t *testing.T) {
	a, d, _ := notifyAPI(t)
	a.probeGate = newProbeConcurrencyGate(config.DefaultProbeConcurrency)
	ctx := t.Context()
	name := "api-health-" + newUUID()
	_, _ = d.Pool.Exec(ctx, `DELETE FROM shippin_platform_service_observations WHERE service_id IN(SELECT id FROM shippin_platform_services WHERE name=$1)`, name)
	_, _ = d.Pool.Exec(ctx, `DELETE FROM shippin_platform_services WHERE name=$1`, name)
	c := &caller{id: &auth.Identity{Sub: "ops-" + newUUID()}}
	body := func(revision int64) string {
		b, _ := json.Marshal(map[string]any{"name": name, "kind": "custom", "url": "http://127.0.0.1:1/health", "timeout_seconds": 1, "interval_seconds": 30, "revision": revision, "reason": "test platform health lifecycle"})
		return string(b)
	}
	r := httptest.NewRequest("POST", "/v1/operator/platform-services", strings.NewReader(body(0)))
	w := httptest.NewRecorder()
	a.opSavePlatformService(true)(w, r, c)
	if w.Code != 200 {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var created struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	list := httptest.NewRecorder()
	a.opPlatformServices(list, httptest.NewRequest("GET", "/v1/operator/platform-services", nil), c)
	if list.Code != 200 {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	readState := func() string {
		var payload struct {
			Items []struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"items"`
		}
		if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		for _, item := range payload.Items {
			if item.ID == created.ID {
				return item.State
			}
		}
		return "missing"
	}
	if state := readState(); state != "unknown" {
		t.Fatalf("unobserved service state=%q, want unknown", state)
	}
	var n int
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM shippin_platform_service_observations WHERE service_id=$1::uuid`, created.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("read generated an observation: count=%d err=%v", n, err)
	}
	check := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/v1/operator/platform-services/"+created.ID+"/check", strings.NewReader("{}"))
	request.SetPathValue("serviceId", created.ID)
	a.opCheckPlatformService(check, request, c)
	if check.Code != 200 {
		t.Fatalf("check status=%d body=%s", check.Code, check.Body.String())
	}
	var healthy bool
	if err := d.Pool.QueryRow(ctx, `SELECT healthy FROM shippin_platform_service_observations WHERE service_id=$1::uuid ORDER BY observed_at DESC LIMIT 1`, created.ID).Scan(&healthy); err != nil {
		t.Fatal(err)
	}
	if healthy {
		t.Fatal("unreachable local target was recorded healthy")
	}
	history := httptest.NewRecorder()
	request = httptest.NewRequest("GET", "/v1/operator/platform-services/"+created.ID+"/observations", nil)
	request.SetPathValue("serviceId", created.ID)
	a.opPlatformServiceObservations(history, request, c)
	var historyBody struct {
		Items []struct {
			ID      string `json:"id"`
			Healthy bool   `json:"healthy"`
		} `json:"items"`
	}
	if history.Code != 200 || json.Unmarshal(history.Body.Bytes(), &historyBody) != nil || len(historyBody.Items) != 1 || historyBody.Items[0].Healthy {
		t.Fatalf("observation history did not preserve failed probe: status=%d body=%s", history.Code, history.Body.String())
	}
	list = httptest.NewRecorder()
	a.opPlatformServices(list, httptest.NewRequest("GET", "/v1/operator/platform-services", nil), c)
	if state := readState(); state != "down" {
		t.Fatalf("failed target state=%q, want down", state)
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE shippin_platform_service_observations SET observed_at=now()-interval '61 seconds' WHERE service_id=$1::uuid`, created.ID); err != nil {
		t.Fatal(err)
	}
	list = httptest.NewRecorder()
	a.opPlatformServices(list, httptest.NewRequest("GET", "/v1/operator/platform-services", nil), c)
	if state := readState(); state != "stale" {
		t.Fatalf("old observation state=%q, want stale", state)
	}
	update := httptest.NewRecorder()
	request = httptest.NewRequest("PUT", "/v1/operator/platform-services/"+created.ID, strings.NewReader(body(1)))
	request.SetPathValue("serviceId", created.ID)
	a.opSavePlatformService(false)(update, request, c)
	if update.Code != 200 {
		t.Fatalf("update status=%d body=%s", update.Code, update.Body.String())
	}
	stale := httptest.NewRecorder()
	request = httptest.NewRequest("PUT", "/v1/operator/platform-services/"+created.ID, strings.NewReader(body(1)))
	request.SetPathValue("serviceId", created.ID)
	a.opSavePlatformService(false)(stale, request, c)
	if stale.Code != 409 {
		t.Fatalf("stale update status=%d", stale.Code)
	}
	disable := httptest.NewRecorder()
	request = httptest.NewRequest("DELETE", "/v1/operator/platform-services/"+created.ID, strings.NewReader(`{"revision":2,"reason":"remove from active monitoring"}`))
	request.SetPathValue("serviceId", created.ID)
	a.opDisablePlatformService(disable, request, c)
	if disable.Code != 204 {
		t.Fatalf("disable status=%d body=%s", disable.Code, disable.Body.String())
	}
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM shippin_platform_service_observations WHERE service_id=$1::uuid`, created.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("disable discarded observation history: count=%d err=%v", n, err)
	}
}

func TestPlatformHealthMutationsRejectMalformedServiceIDs(t *testing.T) {
	a := &API{}
	c := &caller{id: &auth.Identity{Sub: "operator"}}
	check := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/v1/operator/platform-services/not-a-uuid/check", strings.NewReader("{}"))
	request.SetPathValue("serviceId", "not-a-uuid")
	a.opCheckPlatformService(check, request, c)
	if check.Code != 400 {
		t.Fatalf("malformed check ID status=%d", check.Code)
	}
	disable := httptest.NewRecorder()
	request = httptest.NewRequest("DELETE", "/v1/operator/platform-services/not-a-uuid", strings.NewReader(`{"revision":1,"reason":"test malformed identifier"}`))
	request.SetPathValue("serviceId", "not-a-uuid")
	a.opDisablePlatformService(disable, request, c)
	if disable.Code != 400 {
		t.Fatalf("malformed disable ID status=%d", disable.Code)
	}
}
