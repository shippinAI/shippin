package api

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"shippin.cloud/console/internal/approval"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hv/proxmox"
	"shippin.cloud/console/internal/servers"
)

func TestInfraRoutesNeedTheInfraRole(t *testing.T) {
	for _, op := range []string{"", "op_support", "op_superadmin", "org_owner"} {
		a := infraAPI(t, op, nil)
		mux := http.NewServeMux()
		a.infraRoutes(mux)
		for _, rt := range a.infraRouteTable() {
			method, path, _ := strings.Cut(rt.pattern, " ")
			path = strings.NewReplacer("{id}", "s1", "{guest}", "100", "{verb}", "start", "{name}", "nightly", "{storage}", "local", "{upid}", "UPID:n1:1").Replace(path)
			req := httptest.NewRequest(method, path, strings.NewReader("{}"))
			req.Header.Set("Authorization", "Bearer test")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("op=%q %s: got %d, want 403", op, rt.pattern, rec.Code)
			}
		}
	}
}

func TestInfraRoutesNeedAToken(t *testing.T) {
	a := infraAPI(t, "op_infra", nil)
	mux := http.NewServeMux()
	a.infraRoutes(mux)
	for _, rt := range a.infraRouteTable() {
		method, path, _ := strings.Cut(rt.pattern, " ")
		path = strings.ReplaceAll(path, "{id}", "s1")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without token: got %d, want 401", rt.pattern, rec.Code)
		}
	}
}

func TestInfraGuestLifecycleReadsBack(t *testing.T) {
	var mu sync.Mutex
	status := "stopped"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "PVEAPIToken=") {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api2/json/version":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"version": "8.2.2", "release": "8.2"}})
		case r.URL.Path == "/api2/json/cluster/resources":
			mu.Lock()
			st := status
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"vmid": 100, "name": "app", "type": "lxc", "status": st, "node": "n1"},
			}})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/status/start"):
			mu.Lock()
			status = "running"
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "UPID:n1:00000001:00000002:00000003:vzstart:100:token@pve:"})
		case strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/status"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "stopped", "exitstatus": "OK", "type": "vzstart", "upid": "UPID:n1:00000001:00000002:00000003:vzstart:100:token@pve:"}})
		case strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/log"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"n": 1, "t": "TASK OK"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	const secret = "s3cret-token-value"
	cl, err := proxmox.New(proxmox.Endpoint{BaseURL: srv.URL, Node: "n1", TokenID: "id!tok", CAPEM: pemBytes}, func() (string, error) { return secret, nil })
	if err != nil {
		t.Fatal(err)
	}
	id := "019a0000-0000-7000-8000-0000000000aa"
	dir := &memHV{items: []servers.Endpoint{{ID: id, Slug: "lab", Label: "Lab", Kind: "proxmox", BaseURL: "https://pve.example:8006", Node: "n1", TokenID: "id!tok", TokenEnv: "SHIPPIN_HV_TOKEN"}}}
	a := infraAPI(t, "op_infra", dir)
	a.openPVE = func(servers.Endpoint) (*proxmox.Client, error) { return cl, nil }
	mux := http.NewServeMux()
	a.infraRoutes(mux)

	rec := infraDo(mux, http.MethodGet, "/v1/infra/servers", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("list %d %s", rec.Code, rec.Body.String())
	}
	rec = infraDo(mux, http.MethodGet, "/v1/infra/servers/registry", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"slug":"lab"`) || strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), "token_env") {
		t.Fatalf("registry %d %s", rec.Code, rec.Body.String())
	}
	rec = infraDo(mux, http.MethodGet, "/v1/infra/servers/"+id+"/guests", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"100"`) {
		t.Fatalf("guests %d %s", rec.Code, rec.Body.String())
	}
	rec = infraDo(mux, http.MethodPost, "/v1/infra/servers/"+id+"/guests/100/status/start", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"running"`) {
		t.Fatalf("start %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatal("response included the token secret")
	}
	upid := "UPID:n1:00000001:00000002:00000003:vzstart:100:token@pve:"
	rec = infraDo(mux, http.MethodGet, "/v1/infra/servers/"+id+"/tasks/"+upid, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"done":true`) || !strings.Contains(rec.Body.String(), "TASK OK") {
		t.Fatalf("task %d %s", rec.Code, rec.Body.String())
	}
	acts := a.staffDB.(*fakeStore).actions()
	if len(acts) != 1 || acts[0] != "hv.lifecycle" {
		t.Fatalf("audit %v", acts)
	}
	for _, e := range a.staffDB.(*fakeStore).audit {
		if strings.Contains(e.Detail, secret) || strings.Contains(e.Detail, "id!tok") {
			t.Fatalf("audit detail leaked credentials: %s", e.Detail)
		}
	}
}

func TestInfraWaitsOnTheBoardAndRejectsPlainHTTP(t *testing.T) {
	id := "019a0000-0000-7000-8000-0000000000aa"
	dir := &memHV{items: []servers.Endpoint{{ID: id, Kind: "proxmox", BaseURL: "https://pve.example:8006", Node: "n1", TokenID: "id!tok", TokenEnv: "SHIPPIN_HV_TOKEN"}}}
	a := infraAPI(t, "op_infra", dir)
	a.cfg.Approval.Board.Driver = "hub"
	a.cfg.Approval.Board.OrgID = "ops"
	a.hvBoard = stubBoard{st: "pending"}
	mux := http.NewServeMux()
	a.infraRoutes(mux)
	rec := infraDo(mux, http.MethodPost, "/v1/infra/servers/"+id+"/guests/100/status/start", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"waiting_on_board"`) || !strings.Contains(rec.Body.String(), "hv.lifecycle:"+id+":100:start") {
		t.Fatalf("board %d %s", rec.Code, rec.Body.String())
	}
	rec = infraDo(mux, http.MethodPut, "/v1/infra/servers/"+id, `{"kind":"proxmox","base_url":"http://pve.example:8006","node":"n1","token_id":"id!tok","token_env":"SHIPPIN_HV_TOKEN"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("http url %d %s", rec.Code, rec.Body.String())
	}
}

type stubBoard struct{ st string }

func (s stubBoard) Status(context.Context, string, string) (string, error) { return s.st, nil }
func (s stubBoard) Post(context.Context, string, string, string, string, string) (string, error) {
	return s.st, nil
}

type memHV struct {
	mu    sync.Mutex
	items []servers.Endpoint
}

func (m *memHV) Endpoints(context.Context) ([]servers.Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]servers.Endpoint{}, m.items...), nil
}
func (m *memHV) Endpoint(_ context.Context, id string) (servers.Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.items {
		if e.ID == id {
			return e, nil
		}
	}
	return servers.Endpoint{}, servers.ErrNotFound
}
func (m *memHV) Registry(context.Context) ([]servers.Ref, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]servers.Ref, len(m.items))
	for i, e := range m.items {
		out[i] = servers.Ref{ID: e.ID, Slug: e.Slug, Label: e.Label, Kind: e.Kind}
	}
	return out, nil
}
func (m *memHV) CreateEndpoint(_ context.Context, e servers.Endpoint, _ string) (servers.Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.items {
		if x.Slug == e.Slug {
			return servers.Endpoint{}, servers.ErrSlugTaken
		}
	}
	e.ID = "new-" + e.Slug
	m.items = append(m.items, e)
	return e, nil
}
func (m *memHV) SetEndpoint(_ context.Context, e servers.Endpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].ID == e.ID {
			e.Slug, e.Label = m.items[i].Slug, m.items[i].Label
			m.items[i] = e
			return nil
		}
	}
	return servers.ErrNotFound
}

func infraAPI(t *testing.T, op string, dir hvDirectory) *API {
	t.Helper()
	cfg := testCfg()
	cfg.Roles.Infra = "op_infra"
	cfg.Approval = approval.Policy{
		Enabled: true,
		Board:   approval.BoardConfig{Driver: "off"},
		Rules: []approval.Rule{{
			ID: "hv", Actions: []string{hvAction}, ApproverRoles: []string{"op_infra"}, NeedsApproval: true,
		}},
	}
	return &API{
		cfg: cfg, flow: &auth.Flow{}, staffDB: newFakeStore("legacy"), hvServers: dir,
		resolveCaller: func(http.ResponseWriter, *http.Request) (*caller, bool) {
			return &caller{id: &auth.Identity{Sub: "u1", Name: "Ada"}, op: op}, true
		},
	}
}

const hvAction = "hv.lifecycle"

func TestTenantGuestsStayInsideTheOrg(t *testing.T) {
	var mu sync.Mutex
	status := "stopped"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/cluster/resources":
			mu.Lock()
			st := status
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"vmid": 100, "name": "app", "type": "lxc", "status": st, "node": "n1"},
			}})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/status/start"):
			mu.Lock()
			status = "running"
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "UPID:n1:1:2:3:vzstart:100:u@pve:"})
		case strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/status"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "stopped", "exitstatus": "OK", "type": "vzstart"}})
		case strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/log"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"t": "TASK OK"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	cl, err := proxmox.New(proxmox.Endpoint{BaseURL: srv.URL, Node: "n1", TokenID: "id!tok", CAPEM: pemBytes}, func() (string, error) { return "secret", nil })
	if err != nil {
		t.Fatal(err)
	}
	const serverID = "019a0000-0000-7000-8000-0000000000aa"
	dir := &memHV{items: []servers.Endpoint{{ID: serverID, Kind: "proxmox", BaseURL: "https://pve.example:8006", Node: "n1", TokenID: "id!tok", TokenEnv: "SHIPPIN_HV_TOKEN"}}}
	owner := infraAPI(t, "", dir)
	owner.cfg.Servers.Enabled = true
	owner.cfg.Approval.Rules[0].ApproverRoles = []string{"org_owner"}
	owner.openPVE = func(servers.Endpoint) (*proxmox.Client, error) { return cl, nil }
	owner.hvVisible = func(_ context.Context, tenant, id string) (servers.Endpoint, error) {
		if tenant == "t1" && id == serverID {
			return servers.Endpoint{ID: id, Kind: "proxmox", BaseURL: "https://pve.example:8006", Node: "n1", TokenID: "id!tok", TokenEnv: "SHIPPIN_HV_TOKEN"}, nil
		}
		return servers.Endpoint{}, servers.ErrNotFound
	}
	owner.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) {
		return &caller{id: &auth.Identity{Sub: "u1", Name: "Ada"}, managers: []string{"org_owner", "org_admin"},
			tenants: []tenantView{
				{Tenant: db.Tenant{ID: "t1"}, Roles: []string{"org_owner"}},
				{Tenant: db.Tenant{ID: "t2"}, Roles: []string{"org_owner"}},
			}}, true
	}
	mux := http.NewServeMux()
	owner.infraRoutes(mux)
	rec := infraDo(mux, http.MethodGet, "/v1/tenants/t1/servers/"+serverID+"/guests", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"100"`) {
		t.Fatalf("owner list %d %s", rec.Code, rec.Body.String())
	}
	rec = infraDo(mux, http.MethodGet, "/v1/tenants/t2/servers/"+serverID+"/guests", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("other org %d %s", rec.Code, rec.Body.String())
	}
	rec = infraDo(mux, http.MethodPost, "/v1/tenants/t1/servers/"+serverID+"/guests/100/status/start", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"running"`) {
		t.Fatalf("owner start %d %s", rec.Code, rec.Body.String())
	}
	member := infraAPI(t, "", dir)
	member.cfg.Servers.Enabled = true
	member.openPVE = owner.openPVE
	member.hvVisible = owner.hvVisible
	member.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) {
		return &caller{id: &auth.Identity{Sub: "u2", Name: "Bea"}, managers: []string{"org_owner", "org_admin"},
			tenants: []tenantView{{Tenant: db.Tenant{ID: "t1"}, Roles: []string{"org_member"}}}}, true
	}
	mux = http.NewServeMux()
	member.infraRoutes(mux)
	rec = infraDo(mux, http.MethodGet, "/v1/tenants/t1/servers/"+serverID+"/guests", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("member list %d %s", rec.Code, rec.Body.String())
	}
	rec = infraDo(mux, http.MethodPost, "/v1/tenants/t1/servers/"+serverID+"/guests/100/status/start", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member start %d %s", rec.Code, rec.Body.String())
	}
}

func infraDo(mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}
