package api

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/servers"
)

var usableKey = strings.Repeat("ab", 32)

func usableAPI(t *testing.T, dir hvDirectory) *API {
	t.Helper()
	a := attachPolicyAPI(t, "op_infra", dir)
	a.cfg.SessionKey = usableKey
	return a
}

func infraCall(t *testing.T, a *API, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	a.infraRoutes(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set(noteHeader, "A test of this change")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// fakePVE answers like Proxmox with its own certificate, which nothing trusts yet.
func fakePVE(t *testing.T, status *string, sawAuth *string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if sawAuth != nil {
			*sawAuth = r.Header.Get("Authorization")
		}
		switch {
		case r.URL.Path == "/api2/json/version":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"version": "9.2.11"}})
		case r.URL.Path == "/api2/json/nodes":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"node": "pve1"}}})
		case r.URL.Path == "/api2/json/nodes/pve1/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"pveversion": "pve-manager/9.2.11/x", "kversion": "Linux 7.0.14-pve #1", "uptime": 60, "cpuinfo": map[string]any{"model": "Xeon", "cpus": 8}, "memory": map[string]any{"total": 100, "used": 40}}})
		case r.URL.Path == "/api2/json/cluster/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"type": "node", "name": "pve1", "online": 1, "local": 1}}})
		case strings.HasSuffix(r.URL.Path, "/status/current"):
			st := "running"
			if status != nil {
				st = *status
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": st}})
		case r.URL.Path == "/api2/json/cluster/resources":
			st := "running"
			if status != nil {
				st = *status
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"vmid": 100, "name": "web", "type": "lxc", "status": st, "node": "pve1"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFounderReachesInfrastructure(t *testing.T) {
	a := infraAPI(t, "op_superadmin", &memHV{})
	if a.infraAllowed(&caller{id: &auth.Identity{Sub: "idp"}, op: "op_superadmin"}) {
		t.Fatal("a platform admin alone is not implied")
	}
	if !a.infraAllowed(&caller{id: &auth.Identity{Sub: "idp"}, op: "op_superadmin", founder: true}) {
		t.Fatal("the founder role passes every check, including this one")
	}
}

func TestConnectionTestOffersAnUntrustedCertificateThenWorksOnceTrusted(t *testing.T) {
	srv := fakePVE(t, nil, nil)
	dir := &memHV{}
	a := usableAPI(t, dir)
	body := func(extra string) string {
		return fmt.Sprintf(`{"kind":"proxmox","base_url":%q,"token_id":"u@pam!t","token_secret":"s3cret"%s}`, srv.URL, extra)
	}
	rec := infraCall(t, a, http.MethodPost, "/v1/infra/servers/test", body(""))
	var first connectResult
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if first.OK || first.Certificate == nil || first.Certificate.PEM == "" || !strings.Contains(first.Certificate.Fingerprint, ":") {
		t.Fatalf("an unknown certificate must be offered for review, not accepted: %+v", first)
	}
	pemJSON, _ := json.Marshal(first.Certificate.PEM)
	rec = infraCall(t, a, http.MethodPost, "/v1/infra/servers/test", body(`,"trusted_cert":`+string(pemJSON)))
	var second connectResult
	_ = json.Unmarshal(rec.Body.Bytes(), &second)
	if !second.OK || second.Version != "9.2.11" || len(second.Nodes) != 1 || second.Nodes[0] != "pve1" {
		t.Fatalf("%+v", second)
	}
	if len(dir.items) != 0 {
		t.Fatal("a test must store nothing")
	}
}

func TestProxmoxSecretIsSealedUsedAndNeverReturned(t *testing.T) {
	var sawAuth string
	srv := fakePVE(t, nil, &sawAuth)
	dir := &memHV{}
	a := usableAPI(t, dir)
	cert := hv.DescribeCertificate(srv.Certificate()).PEM
	pemJSON, _ := json.Marshal(cert)
	rec := infraCall(t, a, http.MethodPost, "/v1/infra/servers", fmt.Sprintf(`{"kind":"proxmox","label":"Lab","slug":"lab","base_url":%q,"token_id":"u@pam!t","token_secret":"s3cret-value","trusted_cert":%s}`, srv.URL, pemJSON))
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "s3cret-value") {
		t.Fatalf("secret in the response: %s", rec.Body)
	}
	stored := dir.items[0]
	if stored.Node != "pve1" {
		t.Fatalf("a single node should be found, got %q", stored.Node)
	}
	if len(stored.SealedSecret) == 0 || strings.Contains(string(stored.SealedSecret), "s3cret-value") || stored.Secret != "" {
		t.Fatalf("secret not sealed: %+v", stored)
	}
	ov := overviewOf(t, a)
	body, _ := json.Marshal(ov)
	if strings.Contains(string(body), "s3cret-value") {
		t.Fatal("secret in the overview")
	}
	n := providerOf(t, ov, "proxmox")["nodes"].([]any)[0].(map[string]any)
	if n["online"] != true || n["version"] != "9.2.11" {
		t.Fatalf("node %v", n)
	}
	if !strings.HasSuffix(sawAuth, "=s3cret-value") {
		t.Fatalf("the sealed secret should reach the API: %q", sawAuth)
	}
	d := n["details"].(map[string]any)
	if d["kernel"] != "7.0.14-pve" || d["cpus"] != 8.0 || d["mem_total_bytes"] != 100.0 || d["release"] != "pve-manager/9.2.11/x" {
		t.Fatalf("details %v", d)
	}
	// Re-attaching without a new secret keeps the one stored.
	rec = infraCall(t, a, http.MethodPut, "/v1/infra/servers/"+stored.ID, fmt.Sprintf(`{"kind":"proxmox","base_url":%q,"token_id":"u@pam!t","node":"pve1","trusted_cert":%s}`, srv.URL, pemJSON))
	if rec.Code != http.StatusOK || len(dir.items[0].SealedSecret) == 0 {
		t.Fatalf("re-attach lost the secret: %d %s", rec.Code, rec.Body)
	}
}

func TestProxmoxWithoutAnySecretIsRefused(t *testing.T) {
	rec := infraCall(t, usableAPI(t, &memHV{}), http.MethodPost, "/v1/infra/servers", `{"kind":"proxmox","label":"Lab","slug":"lab","base_url":"https://x.invalid","token_id":"u@pam!t"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestIncusTrustTokenAddsAHostFromThePanelAlone(t *testing.T) {
	var mu sync.Mutex
	trusted := map[string]bool{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		peer := ""
		if len(r.TLS.PeerCertificates) > 0 {
			peer = hv.Fingerprint(r.TLS.PeerCertificates[0])
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/certificates":
			trusted[peer] = true
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/1.0":
			auth := "untrusted"
			if trusted[peer] {
				auth = "trusted"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"auth": auth, "environment": map[string]any{"server_version": "6.0.4", "server_name": "plat", "kernel_version": "6.12"}}})
		case r.URL.Path == "/1.0/instances":
			if !trusted[peer] {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": []map[string]string{{"name": "vm1", "status": "Running", "type": "container"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	srv.TLS = &tls.Config{ClientAuth: tls.RequestClientCert}
	srv.StartTLS()
	defer srv.Close()
	raw, _ := json.Marshal(map[string]any{"client_name": "panel", "fingerprint": hv.Fingerprint(srv.Certificate()), "addresses": []string{strings.TrimPrefix(srv.URL, "https://")}, "secret": "x"})
	token := base64.StdEncoding.EncodeToString(raw)
	dir := &memHV{}
	a := usableAPI(t, dir)

	rec := infraCall(t, a, http.MethodPost, "/v1/infra/servers/test", fmt.Sprintf(`{"kind":"incus","trust_token":%q}`, token))
	var tested connectResult
	_ = json.Unmarshal(rec.Body.Bytes(), &tested)
	if !tested.OK || !strings.Contains(tested.Detail, "matches the token") {
		t.Fatalf("%+v", tested)
	}
	mu.Lock()
	spent := len(trusted)
	mu.Unlock()
	if spent != 0 {
		t.Fatal("a test must not spend the one-time token")
	}

	rec = infraCall(t, a, http.MethodPost, "/v1/infra/servers", fmt.Sprintf(`{"kind":"incus","label":"Lab","slug":"lab","trust_token":%q}`, token))
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "PRIVATE KEY") || strings.Contains(rec.Body.String(), token) {
		t.Fatalf("secret material in the response: %s", rec.Body)
	}
	stored := dir.items[0]
	if stored.BaseURL != srv.URL || stored.ClientCert == "" || len(stored.SealedKey) == 0 || strings.Contains(string(stored.SealedKey), "PRIVATE KEY") {
		t.Fatalf("%+v", stored)
	}
	n := providerOf(t, overviewOf(t, a), "incus")["nodes"].([]any)[0].(map[string]any)
	if n["online"] != true || n["version"] != "6.0.4" || n["guests"].(map[string]any)["total"] != 1.0 {
		t.Fatalf("the stored credential should work: %v", n)
	}
}

func TestABadTrustTokenAddsNothing(t *testing.T) {
	dir := &memHV{}
	rec := infraCall(t, usableAPI(t, dir), http.MethodPost, "/v1/infra/servers", `{"kind":"incus","label":"Lab","slug":"lab","trust_token":"not-a-token"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "trust token") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if len(dir.items) != 0 {
		t.Fatal("wrote a row")
	}
}

func TestOperationOutcomeTellsARefusalFromSilence(t *testing.T) {
	if st, _ := operationOutcome(nil); st != servers.OpSucceeded {
		t.Errorf("nil: %s", st)
	}
	if st, d := operationOutcome(errors.New("hypervisor reports stopped")); st != servers.OpFailed || d == "" {
		t.Errorf("refusal: %s %q", st, d)
	}
	for name, err := range map[string]error{
		"timeout":   context.DeadlineExceeded,
		"transport": &url.Error{Op: "Post", URL: "https://x", Err: errors.New("connection reset")},
		"wrapped":   fmt.Errorf("start: %w", context.DeadlineExceeded),
	} {
		if st, d := operationOutcome(err); st != servers.OpUnknown || !strings.Contains(d, "before trying again") {
			t.Errorf("%s: %s %q", name, st, d)
		}
	}
}

// stubDriver changes state on command, for lifecycle tests with no network.
type stubDriver struct {
	hv.Driver
	mu     sync.Mutex
	guests map[string]*hv.Guest
	calls  []string
	fail   map[string]error
}

func (s *stubDriver) Guest(_ context.Context, id string) (hv.Guest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if g, ok := s.guests[id]; ok {
		return *g, nil
	}
	return hv.Guest{}, fmt.Errorf("guest %s not in inventory", id)
}
func (s *stubDriver) change(id, verb, status string) (hv.Guest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, verb+":"+id)
	if err := s.fail[id]; err != nil {
		return hv.Guest{}, err
	}
	s.guests[id].Status = status
	return *s.guests[id], nil
}
func (s *stubDriver) Shutdown(_ context.Context, id string) (hv.Guest, error) {
	return s.change(id, "shutdown", "stopped")
}
func (s *stubDriver) Stop(_ context.Context, id string) (hv.Guest, error) {
	return s.change(id, "stop", "stopped")
}

func waitForOps(t *testing.T, dir *memHV, want int) []servers.Operation {
	t.Helper()
	for i := 0; i < 200; i++ {
		ops, _ := dir.Operations(context.Background(), "", 0)
		done := 0
		for _, o := range ops {
			if o.Status != servers.OpQueued && o.Status != servers.OpRunning {
				done++
			}
		}
		if done == want {
			return ops
		}
		time.Sleep(5 * time.Millisecond)
	}
	ops, _ := dir.Operations(context.Background(), "", 0)
	t.Fatalf("operations never settled: %+v", ops)
	return nil
}

func TestStopOneRecordsEachGuestAndSparesThePanel(t *testing.T) {
	t.Setenv("SHIPPIN_SELF_GUEST", "panel-vm")
	d := &stubDriver{guests: map[string]*hv.Guest{
		"a":        {ID: "a", Name: "a", Status: "running"},
		"b":        {ID: "b", Name: "b", Status: "stopped"},
		"c":        {ID: "c", Name: "c", Status: "running"},
		"panel-vm": {ID: "panel-vm", Name: "panel-vm", Status: "running"},
		"gone":     {ID: "gone", Name: "gone", Status: "running"},
	}, fail: map[string]error{"gone": errors.New("hypervisor reports running")}}
	dir := &memHV{items: []servers.Endpoint{{ID: "s1", Kind: "incus"}}}
	a := usableAPI(t, dir)
	ctx := context.Background()
	for _, g := range []string{"a", "b", "panel-vm", "gone", "c", "missing"} {
		id, _ := dir.StartOperation(ctx, servers.Operation{ServerID: "s1", Guest: g, Verb: "shutdown", Status: servers.OpQueued})
		a.stopOne(ctx, d, id, g, false)
	}
	ops := waitForOps(t, dir, 6)
	got := map[string]string{}
	for _, o := range ops {
		got[o.Guest] = o.Status
	}
	want := map[string]string{"a": servers.OpSucceeded, "b": servers.OpSucceeded, "panel-vm": servers.OpSkipped, "gone": servers.OpFailed, "c": servers.OpSucceeded, "missing": servers.OpFailed}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %s, want %s", k, got[k], v)
		}
	}
	if d.guests["panel-vm"].Status != "running" {
		t.Fatal("the panel's own guest was stopped")
	}
	if strings.Join(d.calls, ",") != "shutdown:a,shutdown:gone,shutdown:c" {
		t.Fatalf("order and skips: %v", d.calls)
	}
}

func TestRefuseSelfNeedsConfirmationAndThePermission(t *testing.T) {
	t.Setenv("SHIPPIN_SELF_GUEST", "panel-vm")
	d := &stubDriver{guests: map[string]*hv.Guest{"panel-vm": {ID: "panel-vm", Name: "panel-vm", Status: "running"}, "other": {ID: "other", Name: "other", Status: "running"}}}
	a := enforcedInfraAPI(t, &memHV{}, "infra.guests.power")
	limited := staffCaller("infra.guests.power")
	full := staffCaller("infra.guests.power", "infra.self")
	call := func(c *caller, guest, verb, query string) (bool, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		return a.refuseSelf(rec, httptest.NewRequest(http.MethodPost, "/x"+query, nil), c, d, guest, verb), rec
	}
	if refused, rec := call(full, "panel-vm", "stop", ""); !refused || rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "self_guest") {
		t.Fatalf("unconfirmed: %v %d %s", refused, rec.Code, rec.Body)
	}
	if refused, rec := call(limited, "panel-vm", "stop", "?confirm=self"); !refused || rec.Code != http.StatusForbidden {
		t.Fatalf("confirming must not be enough without infra.self: %v %d", refused, rec.Code)
	}
	if refused, _ := call(full, "panel-vm", "stop", "?confirm=self"); refused {
		t.Fatal("confirmed with the permission: should proceed")
	}
	if refused, _ := call(limited, "panel-vm", "start", ""); refused {
		t.Fatal("starting it is harmless")
	}
	if refused, _ := call(limited, "other", "stop", ""); refused {
		t.Fatal("another guest is not the panel")
	}
}

func TestMarkSelfFlagsOnlyThePanel(t *testing.T) {
	t.Setenv("SHIPPIN_SELF_GUEST", "Panel-VM")
	got := markSelf([]hv.Guest{{ID: "100", Name: "panel-vm"}, {ID: "101", Name: "other"}})
	if !got[0].Self || got[1].Self {
		t.Fatalf("%+v", got)
	}
}

func TestAChangeWithNoRecordDoesNotRun(t *testing.T) {
	dir := &memHV{failOps: true}
	a := usableAPI(t, dir)
	ran := false
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	_, err := a.tracked(r, &caller{id: &auth.Identity{Sub: "u"}}, "s1", "g", "stop", func(context.Context) (hv.Guest, error) {
		ran = true
		return hv.Guest{}, nil
	})
	if err == nil || ran {
		t.Fatalf("err=%v ran=%v", err, ran)
	}
}

func TestTrackedFinishesEvenIfTheCallerLeaves(t *testing.T) {
	dir := &memHV{}
	a := usableAPI(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest(http.MethodPost, "/x", nil).WithContext(ctx)
	cancel() // the browser went away
	g, err := a.tracked(r, &caller{id: &auth.Identity{Sub: "u"}}, "s1", "g", "stop", func(ctx context.Context) (hv.Guest, error) {
		if ctx.Err() != nil {
			return hv.Guest{}, ctx.Err()
		}
		return hv.Guest{Status: "stopped"}, nil
	})
	if err != nil || g.Status != "stopped" {
		t.Fatalf("%v %+v", err, g)
	}
	if ops, _ := dir.Operations(context.Background(), "", 0); ops[0].Status != servers.OpSucceeded || ops[0].GuestStatus != "stopped" {
		t.Fatalf("%+v", ops)
	}
}

func TestRecoverOperationsSettlesWhatARestartLeftOpen(t *testing.T) {
	dir := &memHV{ops: []servers.Operation{{ID: "1", Status: servers.OpRunning}, {ID: "2", Status: servers.OpQueued}, {ID: "3", Status: servers.OpSucceeded}}}
	usableAPI(t, dir).RecoverOperations(context.Background())
	ops, _ := dir.Operations(context.Background(), "", 0)
	if ops[0].Status != servers.OpUnknown || ops[1].Status != servers.OpUnknown || ops[2].Status != servers.OpSucceeded {
		t.Fatalf("%+v", ops)
	}
}

func TestFleetsGroupHostsWithoutTouchingThem(t *testing.T) {
	dir := &memHV{items: []servers.Endpoint{{ID: "s1", Slug: "a", Label: "A", Kind: "incus", TokenEnv: "T", BaseURL: "https://x.invalid"}, {ID: "s2", Slug: "b", Label: "B", Kind: "proxmox", Node: "n", TokenID: "t", TokenEnv: "T", BaseURL: "https://y.invalid"}}}
	a := usableAPI(t, dir)
	rec := infraCall(t, a, http.MethodPost, "/v1/infra/fleets", `{"label":"Production","description":"Both"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var f servers.Fleet
	_ = json.Unmarshal(rec.Body.Bytes(), &f)
	for _, s := range []string{"s1", "s2"} {
		if rec := infraCall(t, a, http.MethodPut, "/v1/infra/servers/"+s+"/fleet", fmt.Sprintf(`{"fleet_id":%q}`, f.ID)); rec.Code != http.StatusNoContent {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	}
	ov := overviewOf(t, a)
	if fl := ov["fleets"].([]any); len(fl) != 1 || fl[0].(map[string]any)["label"] != "Production" {
		t.Fatalf("%v", ov["fleets"])
	}
	for _, kind := range []string{"incus", "proxmox"} {
		n := providerOf(t, ov, kind)["nodes"].([]any)[0].(map[string]any)
		if n["fleet_id"] != f.ID || n["kind"] != kind {
			t.Fatalf("each stays its own provider in the fleet: %v", n)
		}
	}
	if rec := infraCall(t, a, http.MethodPost, "/v1/infra/fleets", `{"label":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("no name: %d", rec.Code)
	}
	if rec := infraCall(t, a, http.MethodDelete, "/v1/infra/fleets/"+f.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if len(dir.items) != 2 || dir.items[0].FleetID != "" {
		t.Fatalf("deleting a fleet must keep its hosts: %+v", dir.items)
	}
}

func TestOperationsRouteListsTheHistory(t *testing.T) {
	dir := &memHV{ops: []servers.Operation{{ID: "1", Guest: "a", Verb: "stop", Status: servers.OpSucceeded}}}
	rec := infraCall(t, usableAPI(t, dir), http.MethodGet, "/v1/infra/operations", "")
	body, _ := io.ReadAll(rec.Body)
	if rec.Code != http.StatusOK || !strings.Contains(string(body), `"verb":"stop"`) {
		t.Fatalf("%d %s", rec.Code, body)
	}
}
