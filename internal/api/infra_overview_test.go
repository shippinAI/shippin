package api

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/approval"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/servers"
)

func fakeIncus(t *testing.T, statuses ...string) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1.0":
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"environment": map[string]string{"server_version": "6.0"}}})
		case "/1.0/instances":
			items := []map[string]string{}
			for i, st := range statuses {
				items = append(items, map[string]string{"name": "vm" + string(rune('a'+i)), "status": st, "type": "container"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": items})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return srv, ca
}

func incusEndpoint(srv *httptest.Server, ca, id, label string) servers.Endpoint {
	return servers.Endpoint{ID: id, Slug: id, Label: label, Site: "lab", Kind: "incus", BaseURL: srv.URL, TokenEnv: "TEST_INCUS_TOKEN", CAFile: ca}
}

func overviewOf(t *testing.T, a *API) map[string]any {
	t.Helper()
	mux := http.NewServeMux()
	a.infraRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/v1/infra/overview", nil)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set(noteHeader, "A test of this change")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("overview: %d %s", rec.Code, rec.Body)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func providerOf(t *testing.T, ov map[string]any, kind string) map[string]any {
	t.Helper()
	for _, p := range ov["providers"].([]any) {
		if m := p.(map[string]any); m["kind"] == kind {
			return m
		}
	}
	t.Fatalf("no provider %s in %v", kind, ov["providers"])
	return nil
}

func TestOverviewNamesProvidersFromConfigNotCode(t *testing.T) {
	a := infraAPI(t, "op_infra", &memHV{})
	a.cfg.UI = config.UI{Providers: []config.UIProvider{{
		Kind: "proxmox", Label: "Site cluster", Description: "Ours.",
		Fields: []config.UIField{{Name: "base_url", Label: "Address", Help: "Where it lives", Default: "https://x.invalid"}},
	}}}
	ov := overviewOf(t, a)
	p := providerOf(t, ov, "proxmox")
	if p["label"] != "Site cluster" || p["description"] != "Ours." {
		t.Fatalf("label from config: %v", p)
	}
	f := p["fields"].([]any)[0].(map[string]any)
	if f["name"] != "base_url" || f["label"] != "Address" || f["help"] != "Where it lives" || f["default"] != "https://x.invalid" || f["required"] != true {
		t.Fatalf("field: %v", f)
	}
	// No entry: the kind is the name, so nothing here depends on a name compiled into the code.
	if inc := providerOf(t, ov, "incus"); inc["label"] != "incus" || inc["attachable"] != true {
		t.Fatalf("unconfigured provider: %v", inc)
	}
	if len(p["nodes"].([]any)) != 0 {
		t.Fatalf("no host was added: %v", p["nodes"])
	}
}

func TestOverviewHidesAProviderTheSiteTurnedOff(t *testing.T) {
	a := infraAPI(t, "op_infra", &memHV{})
	a.cfg.UI = config.UI{Providers: []config.UIProvider{{Kind: "proxmox", Hidden: true}}}
	for _, p := range overviewOf(t, a)["providers"].([]any) {
		if p.(map[string]any)["kind"] == "proxmox" {
			t.Fatal("hidden provider listed")
		}
	}
}

func TestOverviewShowsLiveCountsAndNeverHidesADeadHost(t *testing.T) {
	t.Setenv("TEST_INCUS_TOKEN", "s3cret-value")
	up, ca := fakeIncus(t, "Running", "Stopped", "Running")
	dead, deadCA := fakeIncus(t)
	dead.Close()
	a := infraAPI(t, "op_infra", &memHV{items: []servers.Endpoint{
		incusEndpoint(up, ca, "up", "Up host"),
		incusEndpoint(dead, deadCA, "down", "Down host"),
	}})
	ov := overviewOf(t, a)
	p := providerOf(t, ov, "incus")
	nodes := map[string]map[string]any{}
	for _, n := range p["nodes"].([]any) {
		m := n.(map[string]any)
		nodes[m["id"].(string)] = m
	}
	u := nodes["up"]
	if u["online"] != true || u["version"] != "6.0" || u["source"] != "api" {
		t.Fatalf("up: %v", u)
	}
	if g := u["guests"].(map[string]any); g["total"] != 3.0 || g["running"] != 2.0 {
		t.Fatalf("guests: %v", g)
	}
	d := nodes["down"]
	if d == nil || d["online"] != false || d["error"] == "" {
		t.Fatalf("a host that does not answer must stay listed with its error: %v", d)
	}
	if tot := ov["totals"].(map[string]any); tot["nodes"] != 2.0 || tot["online"] != 1.0 || tot["guests"] != 3.0 || tot["running"] != 2.0 {
		t.Fatalf("totals: %v", tot)
	}
	if body, _ := json.Marshal(ov); strings.Contains(string(body), "s3cret-value") {
		t.Fatal("token secret in the overview")
	}
}

func TestOverviewListsServersWithNoProvider(t *testing.T) {
	a := infraAPI(t, "op_infra", &memHV{items: []servers.Endpoint{{ID: "s1", Slug: "bare-metal", Label: "Rack 1"}}})
	ov := overviewOf(t, a)
	un := ov["unattached"].([]any)
	if len(un) != 1 || un[0].(map[string]any)["slug"] != "bare-metal" {
		t.Fatalf("unattached: %v", un)
	}
}

func addRequest(t *testing.T, a *API, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	a.infraRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/v1/infra/servers", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set(noteHeader, "A test of this change")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func attachPolicyAPI(t *testing.T, op string, dir hvDirectory) *API {
	t.Helper()
	a := infraAPI(t, op, dir)
	a.cfg.Approval.Rules = append(a.cfg.Approval.Rules, approval.Rule{ID: "attach", Actions: []string{"hv.attach"}, ApproverRoles: []string{"op_infra"}})
	return a
}

func TestAddHostCreatesARowAndAttachesItsAPI(t *testing.T) {
	dir := &memHV{}
	a := attachPolicyAPI(t, "op_infra", dir)
	a.cfg.UI.DefaultSite = "lab"
	rec := addRequest(t, a, `{"kind":"incus","label":"Lab","slug":"lab-1","base_url":"https://incus.invalid:8443","node":"default","token_env":"TEST_INCUS_TOKEN"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if len(dir.items) != 1 || dir.items[0].Kind != "incus" || dir.items[0].Site != "lab" || dir.items[0].Node != "default" {
		t.Fatalf("row: %+v", dir.items)
	}
	if strings.Contains(rec.Body.String(), "s3cret-value") {
		t.Fatalf("secret in response: %s", rec.Body)
	}
}

func TestAddHostRefusesBadInput(t *testing.T) {
	for name, body := range map[string]string{
		"no name":         `{"kind":"incus","slug":"a","base_url":"https://x.invalid","token_env":"T"}`,
		"bad slug":        `{"kind":"incus","label":"A","slug":"Not Valid","base_url":"https://x.invalid","token_env":"T"}`,
		"bad site":        `{"kind":"incus","label":"A","slug":"a","site":"Not Valid","base_url":"https://x.invalid","token_env":"T"}`,
		"unknown kind":    `{"kind":"vmware","label":"A","slug":"a","base_url":"https://x.invalid","token_env":"T"}`,
		"plain http":      `{"kind":"incus","label":"A","slug":"a","base_url":"http://x.invalid","token_env":"T"}`,
		"url with path":   `{"kind":"incus","label":"A","slug":"a","base_url":"https://x.invalid/admin","token_env":"T"}`,
		"incus no auth":   `{"kind":"incus","label":"A","slug":"a","base_url":"https://x.invalid"}`,
		"cert no key":     `{"kind":"incus","label":"A","slug":"a","base_url":"https://x.invalid","cert_file":"/c.pem"}`,
		"proxmox no node": `{"kind":"proxmox","label":"A","slug":"a","base_url":"https://x.invalid","token_id":"u@r!t","token_env":"T"}`,
		"foreign field":   `{"kind":"incus","label":"A","slug":"a","base_url":"https://x.invalid","token_env":"T","token_id":"u@r!t"}`,
		"secret as env":   `{"kind":"incus","label":"A","slug":"a","base_url":"https://x.invalid","token_env":"s3cr3t value"}`,
		"unknown field":   `{"kind":"incus","label":"A","slug":"a","base_url":"https://x.invalid","token_env":"T","password":"x"}`,
	} {
		dir := &memHV{}
		rec := addRequest(t, attachPolicyAPI(t, "op_infra", dir), body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d %s", name, rec.Code, rec.Body)
		}
		if len(dir.items) != 0 {
			t.Errorf("%s: wrote a row", name)
		}
	}
}

func TestAddHostAnswersConflictOnATakenSlug(t *testing.T) {
	dir := &memHV{items: []servers.Endpoint{{ID: "x", Slug: "lab-1"}}}
	rec := addRequest(t, attachPolicyAPI(t, "op_infra", dir), `{"kind":"incus","label":"Lab","slug":"lab-1","base_url":"https://x.invalid","token_env":"T"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestAttachDoesNotNeedABoardCardButGuestChangesStillDo(t *testing.T) {
	dir := &memHV{}
	a := attachPolicyAPI(t, "op_infra", dir)
	a.cfg.Approval.Board.Driver = "hub"
	a.hvBoard = stubBoard{st: "pending"}
	a.cfg.Operator.PlatformOrgID = "org1"
	rec := addRequest(t, a, `{"kind":"incus","label":"Lab","slug":"lab-1","base_url":"https://x.invalid","token_env":"T"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("attach: %d %s", rec.Code, rec.Body)
	}
	mux := http.NewServeMux()
	a.infraRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/v1/infra/servers/"+dir.items[0].ID+"/guests/vm1/status/start", nil)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set(noteHeader, "A test of this change")
	got := httptest.NewRecorder()
	mux.ServeHTTP(got, req)
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "waiting_on_board") {
		t.Fatalf("a guest change must still wait for the board: %d %s", got.Code, got.Body)
	}
}

func TestAttachFallsBackToTheLifecycleRuleOnOlderPolicies(t *testing.T) {
	a := infraAPI(t, "op_infra", &memHV{}) // policy has no hv.attach rule
	if rec := addRequest(t, a, `{"kind":"incus","label":"Lab","slug":"lab-1","base_url":"https://x.invalid","token_env":"T"}`); rec.Code != http.StatusCreated {
		t.Fatalf("op_infra under an older policy: %d %s", rec.Code, rec.Body)
	}
	denied := infraAPI(t, "op_infra", &memHV{})
	denied.cfg.Approval.Rules = denied.cfg.Approval.Rules[:0]
	if rec := addRequest(t, denied, `{"kind":"incus","label":"Lab","slug":"lab-1","base_url":"https://x.invalid","token_env":"T"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("no matching rule must deny: %d %s", rec.Code, rec.Body)
	}
}

func TestDetachClearsTheAttachmentAndKeepsTheRow(t *testing.T) {
	dir := &memHV{items: []servers.Endpoint{{ID: "s1", Slug: "a", Label: "A", Kind: "incus", BaseURL: "https://x.invalid"}}}
	a := attachPolicyAPI(t, "op_infra", dir)
	mux := http.NewServeMux()
	a.infraRoutes(mux)
	req := httptest.NewRequest(http.MethodDelete, "/v1/infra/servers/s1/attachment", nil)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set(noteHeader, "A test of this change")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if len(dir.items) != 1 || dir.items[0].Kind != "" || dir.items[0].BaseURL != "" {
		t.Fatalf("row: %+v", dir.items)
	}
	missing := httptest.NewRequest(http.MethodDelete, "/v1/infra/servers/nope/attachment", nil)
	missing.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, missing)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing: %d", rec.Code)
	}
}

func TestIncusGuestsWorkThroughTheSharedRoutesButProxmoxOnlyCallsAreRefused(t *testing.T) {
	t.Setenv("TEST_INCUS_TOKEN", "s3cret-value")
	srv, ca := fakeIncus(t, "Running")
	a := infraAPI(t, "op_infra", &memHV{items: []servers.Endpoint{incusEndpoint(srv, ca, "s1", "Lab")}})
	mux := http.NewServeMux()
	a.infraRoutes(mux)
	call := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer test")
		req.Header.Set(noteHeader, "A test of this change")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := call(http.MethodGet, "/v1/infra/servers/s1/guests"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"vma"`) {
		t.Fatalf("guests: %d %s", rec.Code, rec.Body)
	}
	if rec := call(http.MethodGet, "/v1/infra/servers/s1/health"); rec.Code != http.StatusOK {
		t.Fatalf("health: %d %s", rec.Code, rec.Body)
	}
	if rec := call(http.MethodGet, "/v1/infra/servers/s1/storage"); rec.Code != http.StatusConflict {
		t.Fatalf("a call only Proxmox has: %d %s", rec.Code, rec.Body)
	}
}

func TestLocalOwnerReachesInfrastructureWithoutBecomingFounder(t *testing.T) {
	a := infraAPI(t, "op_superadmin", &memHV{})
	c := &caller{id: &auth.Identity{Sub: localSubPrefix + "u1"}, op: "op_superadmin"}
	if a.infraAllowed(c) {
		t.Fatal("a platform admin is not implied infra")
	}
	c.installOwner = true
	if !a.infraAllowed(c) {
		t.Fatal("the installation owner has no IdP to grant the role, so must be allowed")
	}
	if c.founder {
		t.Fatal("the owner must not pass every check")
	}
	a.cfg.Identity.FounderRole = "founder"
	if got := approverRoles(a, c); len(got) != 2 || got[0] != "founder" || got[1] != "op_superadmin" {
		t.Fatalf("approver roles: %v", got)
	}
}

func TestParseUIRefusesTyposAndBadValues(t *testing.T) {
	if _, err := parseUI(map[string]any{"navv": map[string]any{}}); err == nil {
		t.Error("unknown field accepted")
	}
	if _, err := parseUI(map[string]any{"text": map[string]any{"bad key": "x"}}); err == nil {
		t.Error("bad text key accepted")
	}
	if _, err := parseUI(map[string]any{"nav": map[string]any{"items": []any{map[string]any{"id": "operator.hosts", "label": "Nodes"}}}}); err != nil {
		t.Errorf("valid: %v", err)
	}
}

func TestStaffReauthWindowOnlyLengthensForTheInstallOwner(t *testing.T) {
	a := infraAPI(t, "op_superadmin", &memHV{})
	a.cfg.Staff.Session.ReauthMinutes = 5
	idp := &caller{id: &auth.Identity{Sub: "idp-user"}, op: "op_superadmin"}
	owner := &caller{id: &auth.Identity{Sub: localSubPrefix + "u1"}, op: "op_superadmin", installOwner: true}
	if got := a.staffReauthWindow(idp); got != 5*time.Minute {
		t.Fatalf("an IdP admin keeps the short step-up: %v", got)
	}
	if got := a.staffReauthWindow(owner); got != 12*time.Hour {
		t.Fatalf("owner: %v", got)
	}
	a.cfg.Staff.Session.LocalReauthMinutes = 30
	if got := a.staffReauthWindow(owner); got != 30*time.Minute {
		t.Fatalf("configured owner window: %v", got)
	}
}

func TestUISettingsListsDriversAndConfig(t *testing.T) {
	a := infraAPI(t, "op_infra", &memHV{})
	a.cfg.UI = config.UI{Text: map[string]string{"infra.title": "Nodes"}}
	rec := httptest.NewRecorder()
	a.uiSettings(rec, httptest.NewRequest(http.MethodGet, "/v1/settings/ui", nil))
	var out config.UI
	var drivers struct {
		Drivers []struct {
			Kind   string `json:"kind"`
			Fields []struct {
				Name string `json:"name"`
			} `json:"fields"`
		} `json:"drivers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &drivers); err != nil {
		t.Fatal(err)
	}
	if out.Text["infra.title"] != "Nodes" {
		t.Fatalf("config missing: %s", rec.Body)
	}
	if len(drivers.Drivers) < 2 || len(drivers.Drivers[0].Fields) == 0 {
		t.Fatalf("drivers missing: %s", rec.Body)
	}
}

// The platform_settings column has a check constraint; a key that fails it can only be found at
// the first save, in front of whoever is trying to change something.
func TestPlatformKeysFitTheColumn(t *testing.T) {
	column := regexp.MustCompile(`^[a-z][a-z0-9_.]{2,80}$`)
	a := infraAPI(t, "op_infra", &memHV{})
	for _, k := range a.platformKeys() {
		if !column.MatchString(k.Key) {
			t.Errorf("%q does not fit platform_settings.key", k.Key)
		}
	}
}

func TestLocalAgentIsAHostOnlyWhenItAnswers(t *testing.T) {
	answering := true
	hostd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !answering {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"metadata": []map[string]string{
			{"name": "one", "status": "Running", "type": "container"}, {"name": "two", "status": "Stopped", "type": "container"},
		}})
	}))
	defer hostd.Close()
	a := infraAPI(t, "op_infra", &memHV{})
	a.host = hostclient.New(hostd.URL)
	a.cfg.Incus = config.Incus{HostdURL: hostd.URL, Project: "platform", HostLabel: "primary"}

	ov := overviewOf(t, a)
	nodes := providerOf(t, ov, "incus")["nodes"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("nodes: %v", nodes)
	}
	n := nodes[0].(map[string]any)
	if n["source"] != "agent" || n["label"] != "primary" || n["scope"] != "platform" || n["online"] != true {
		t.Fatalf("node: %v", n)
	}
	if g := n["guests"].(map[string]any); g["total"] != 2.0 || g["running"] != 1.0 {
		t.Fatalf("guests: %v", g)
	}
	if ov["local_agent"] != nil {
		t.Fatalf("an answering agent is not a note: %v", ov["local_agent"])
	}

	answering = false
	ov = overviewOf(t, a)
	if got := providerOf(t, ov, "incus")["nodes"].([]any); len(got) != 0 {
		t.Fatalf("an agent that does not answer must not count as a host someone added: %v", got)
	}
	if note, _ := ov["local_agent"].(map[string]any); note["label"] != "primary" {
		t.Fatalf("note: %v", ov["local_agent"])
	}
}

func TestNoLocalAgentConfiguredSaysNothing(t *testing.T) {
	a := infraAPI(t, "op_infra", &memHV{})
	if ov := overviewOf(t, a); ov["local_agent"] != nil {
		t.Fatalf("note with no agent configured: %v", ov["local_agent"])
	}
}
