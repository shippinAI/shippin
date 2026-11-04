package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/servers"
)

// staffCaller holds exactly these permissions through a staff role, and nothing else.
func staffCaller(perms ...string) *caller {
	v := &staffView{Perms: map[string]bool{}, Roles: []string{"staff.test"}}
	for _, p := range perms {
		v.Perms[p] = true
	}
	return &caller{id: &auth.Identity{Sub: "u2", Name: "Grace"}, staff: v}
}

// enforcedInfraAPI is a team in enforce mode, whose only caller holds these permissions.
func enforcedInfraAPI(t *testing.T, dir hvDirectory, perms ...string) *API {
	t.Helper()
	a := infraAPI(t, "", dir)
	a.staffDB = newFakeStore("enforce")
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) { return staffCaller(perms...), true }
	return a
}

var infraPathParam = regexp.MustCompile(`\{[^}]+\}`)

func routeRequest(rt infraRoute) (method, path string) {
	method, path, _ = strings.Cut(rt.pattern, " ")
	return method, infraPathParam.ReplaceAllString(path, "019a0000-0000-7000-8000-0000000000aa")
}

func infraPerms() []string {
	out := []string{}
	for _, p := range permCatalog {
		if p.Section == "Infrastructure" {
			out = append(out, p.Key)
		}
	}
	return out
}

// Each permission alone opens exactly the routes that name it, and no route is left open by default.
func TestEachInfraPermissionOpensOnlyItsOwnRoutes(t *testing.T) {
	for _, perm := range infraPerms() {
		a := enforcedInfraAPI(t, &memHV{}, perm)
		mux := http.NewServeMux()
		a.infraRoutes(mux)
		for _, rt := range a.infraRouteTable() {
			method, path := routeRequest(rt)
			rec := infraDo(mux, method, path, "")
			refused := rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "("+rt.perm+")")
			if rt.perm == perm && refused {
				t.Errorf("%s must open %s: %d %s", perm, rt.pattern, rec.Code, rec.Body)
			}
			if rt.perm != perm && !refused {
				t.Errorf("%s must not open %s: %d %s", perm, rt.pattern, rec.Code, rec.Body)
			}
		}
	}
}

func TestEveryWriteRouteOtherThanFleetsAndTheTestNeedsANote(t *testing.T) {
	a := infraAPI(t, "op_infra", &memHV{})
	for _, rt := range a.infraRouteTable() {
		method, _, _ := strings.Cut(rt.pattern, " ")
		noteFree := method == http.MethodGet || rt.perm == "infra.fleets.write" || strings.HasSuffix(rt.pattern, "/servers/test") || strings.HasSuffix(rt.pattern, "/networks/test") || strings.HasSuffix(rt.pattern, "/sync")
		if rt.note == noteFree {
			t.Errorf("%s: note=%v", rt.pattern, rt.note)
		}
	}
}

func TestAChangeWithoutAGoodNoteIsRefusedBeforeAnythingHappens(t *testing.T) {
	store := newFakeStore("legacy")
	a := infraAPI(t, "op_infra", &memHV{})
	a.staffDB = store
	mux := http.NewServeMux()
	a.infraRoutes(mux)
	for _, rt := range a.infraRouteTable() {
		if !rt.note {
			continue
		}
		method, path := routeRequest(rt)
		for name, note := range map[string]string{"none": "", "short": "why", "blank": "      ", "newline": "good%20note%0Ahidden", "bad encoding": "100%", "long": strings.Repeat("x", noteMax+1)} {
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Authorization", "Bearer test")
			if note != "" {
				req.Header.Set(noteHeader, note)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "note_required") {
				t.Errorf("%s with %s note: %d %s", rt.pattern, name, rec.Code, rec.Body)
			}
		}
	}
	if len(store.audit) != 0 {
		t.Fatalf("a refused change must leave no change record: %+v", store.audit)
	}
}

func TestANoteSurvivesInAnyLanguage(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set(noteHeader, "Arr%C3%AAt%20du%20disque%20plein")
	note, ok := changeNote(httptest.NewRecorder(), req)
	if !ok || note != "Arrêt du disque plein" {
		t.Fatalf("%q %v", note, ok)
	}
}

func TestEveryChangeIsRecordedAgainstTheSystemTenantBeforeItRuns(t *testing.T) {
	store := newFakeStore("legacy")
	a := infraAPI(t, "op_infra", &memHV{})
	a.staffDB = store
	mux := http.NewServeMux()
	a.infraRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/v1/infra/fleets", strings.NewReader(`{"label":"Lab"}`))
	req.Header.Set("Authorization", "Bearer test")
	mux.ServeHTTP(httptest.NewRecorder(), req)
	if len(store.audit) == 0 || store.audit[0].Action != "infra.change" || store.audit[0].TenantID != servers.SystemTenantID {
		t.Fatalf("a fleet change is audited too: %+v", store.audit)
	}
	store.failAudit = true
	req = httptest.NewRequest(http.MethodPost, "/v1/infra/fleets", strings.NewReader(`{"label":"Lab 2"}`))
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no record, no change: %d %s", rec.Code, rec.Body)
	}
}

func TestAStaffRoleCountsOnlyInEnforceMode(t *testing.T) {
	a := enforcedInfraAPI(t, &memHV{}, "infra.read")
	ctx := context.Background()
	if !a.infraCan(ctx, staffCaller("infra.read"), "infra.read") {
		t.Fatal("enforce: the role grants it")
	}
	if a.infraCan(ctx, staffCaller("infra.read"), "infra.guests.power") {
		t.Fatal("enforce: a role grants only what it lists")
	}
	shadow := infraAPI(t, "", &memHV{})
	shadow.staffDB = newFakeStore("shadow")
	if shadow.infraCan(ctx, staffCaller("infra.read"), "infra.read") {
		t.Fatal("shadow: nothing is decided by the new roles yet")
	}
}

func TestMeReportsEachInfrastructurePermission(t *testing.T) {
	a := enforcedInfraAPI(t, &memHV{}, "infra.read", "infra.guests.power")
	caps := a.capabilities(httptest.NewRequest(http.MethodGet, "/v1/me", nil), staffCaller("infra.read", "infra.guests.power"))
	raw, _ := json.Marshal(caps)
	if !caps["infra.hosts"] || !caps["infra.guests.power"] || caps["infra.hosts.write"] || caps["infra.self"] {
		t.Fatalf("%s", raw)
	}
}
