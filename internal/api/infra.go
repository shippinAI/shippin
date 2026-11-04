package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"shippin.cloud/console/internal/approval"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hub"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/hv/incus"
	"shippin.cloud/console/internal/hv/proxmox"
	"shippin.cloud/console/internal/servers"
)

// hvDirectory is the server registry's hypervisor columns. Tests substitute a map.
type hvDirectory interface {
	Endpoints(ctx context.Context) ([]servers.Endpoint, error)
	Endpoint(ctx context.Context, id string) (servers.Endpoint, error)
	SetEndpoint(ctx context.Context, e servers.Endpoint) error
	CreateEndpoint(ctx context.Context, e servers.Endpoint, by string) (servers.Endpoint, error)
	Registry(ctx context.Context) ([]servers.Ref, error)
	StartOperation(ctx context.Context, op servers.Operation) (string, error)
	RunOperation(ctx context.Context, id string) error
	FinishOperation(ctx context.Context, id, status, detail, guestStatus string) error
	Operations(ctx context.Context, serverID string, limit int) ([]servers.Operation, error)
	RecentOutcomes(ctx context.Context, serverID string, within time.Duration) (map[string]string, error)
	AbandonOperations(ctx context.Context) (int64, error)
	Fleets(ctx context.Context) ([]servers.Fleet, error)
	SaveFleet(ctx context.Context, f servers.Fleet, by string) (servers.Fleet, error)
	DeleteFleet(ctx context.Context, id string) error
	SetFleet(ctx context.Context, serverID, fleetID string) error
}

type infraRoute struct {
	pattern string
	perm    string // catalog permission; a route without one stops startup
	note    bool   // a change to real infrastructure, which must say why
	h       func(http.ResponseWriter, *http.Request, *caller)
}

func (a *API) infraRoutes(mux Router) {
	for _, rt := range a.infraRouteTable() {
		if !knownPerm(rt.perm) {
			panic("infra route " + rt.pattern + " has no catalog permission (" + rt.perm + ")")
		}
		mux.HandleFunc(rt.pattern, a.flow.Require(a.infraGate(rt)))
	}
	a.tenantHVRoutes(mux)
}

func (a *API) infraRouteTable() []infraRoute {
	return []infraRoute{
		{"GET /v1/infra/overview", "infra.read", false, a.infraOverview},
		{"GET /v1/infra/servers", "infra.read", false, a.infraServers},
		{"GET /v1/infra/networks", "infra.read", false, a.infraNetworkConnections},
		{"GET /v1/infra/networks/{id}/resources", "infra.read", false, a.infraNetworkResources},
		{"POST /v1/infra/networks/{id}/sync", "infra.read", false, a.infraNetworkSync},
		{"POST /v1/infra/networks/test", "infra.network.write", false, a.infraNetworkTest},
		{"POST /v1/infra/networks", "infra.network.write", true, a.infraNetworkCreate},
		{"PUT /v1/infra/networks/{id}", "infra.network.write", true, a.infraNetworkUpdate},
		{"DELETE /v1/infra/networks/{id}", "infra.network.write", true, a.infraNetworkDelete},
		{"POST /v1/infra/servers", "infra.hosts.write", true, a.infraAdd},
		{"POST /v1/infra/servers/test", "infra.hosts.write", false, a.infraTest},
		{"GET /v1/infra/operations", "infra.read", false, a.infraOperations},
		{"POST /v1/infra/servers/{id}/stop-guests", "infra.guests.power", true, a.infraStopGuests},
		{"PUT /v1/infra/servers/{id}/fleet", "infra.fleets.write", false, a.infraSetFleet},
		{"POST /v1/infra/fleets", "infra.fleets.write", false, a.infraFleetCreate},
		{"PUT /v1/infra/fleets/{fleet}", "infra.fleets.write", false, a.infraFleetUpdate},
		{"DELETE /v1/infra/fleets/{fleet}", "infra.fleets.write", false, a.infraFleetDelete},
		{"GET /v1/infra/servers/registry", "infra.read", false, a.infraRegistry},
		{"PUT /v1/infra/servers/{id}", "infra.hosts.write", true, a.infraAttach},
		{"DELETE /v1/infra/servers/{id}/attachment", "infra.hosts.write", true, a.infraDetach},
		{"GET /v1/infra/servers/{id}/health", "infra.read", false, a.infraHealth},
		{"GET /v1/infra/servers/{id}/guests", "infra.read", false, a.infraGuests},
		{"GET /v1/infra/servers/{id}/guests/{guest}", "infra.read", false, a.infraGuest},
		{"GET /v1/infra/servers/{id}/images", "infra.read", false, a.infraImages},
		{"POST /v1/infra/servers/{id}/guests", "infra.guests.manage", true, a.infraCreate},
		{"POST /v1/infra/servers/{id}/guests/{guest}/clone", "infra.guests.manage", true, a.infraClone},
		{"POST /v1/infra/servers/{id}/guests/{guest}/template", "infra.guests.manage", true, a.infraTemplate},
		{"DELETE /v1/infra/servers/{id}/guests/{guest}", "infra.guests.manage", true, a.infraDelete},
		{"PUT /v1/infra/servers/{id}/guests/{guest}/config", "infra.guests.manage", true, a.infraConfig},
		{"POST /v1/infra/servers/{id}/guests/{guest}/resize", "infra.guests.manage", true, a.infraResize},
		{"POST /v1/infra/servers/{id}/guests/{guest}/migrate", "infra.guests.manage", true, a.infraMigrate},
		{"POST /v1/infra/servers/{id}/guests/{guest}/status/{verb}", "infra.guests.power", true, a.infraStatus},
		{"GET /v1/infra/servers/{id}/guests/{guest}/snapshots", "infra.read", false, a.infraSnapshots},
		{"POST /v1/infra/servers/{id}/guests/{guest}/snapshots", "infra.guests.snapshot", true, a.infraSnapshot},
		{"POST /v1/infra/servers/{id}/guests/{guest}/snapshots/{name}/rollback", "infra.guests.restore", true, a.infraRollback},
		{"GET /v1/infra/servers/{id}/storage", "infra.read", false, a.infraStorage},
		{"GET /v1/infra/servers/{id}/storage/{storage}/content", "infra.read", false, a.infraContent},
		{"POST /v1/infra/servers/{id}/storage/{storage}/download-url", "infra.storage.write", true, a.infraDownload},
		{"POST /v1/infra/servers/{id}/storage/{storage}/upload", "infra.storage.write", true, a.infraUpload},
		{"GET /v1/infra/servers/{id}/services", "infra.read", false, a.infraServices},
		{"GET /v1/infra/servers/{id}/network", "infra.read", false, a.infraNetwork},
		{"GET /v1/infra/servers/{id}/firewall/rules", "infra.read", false, a.infraFirewall},
		{"GET /v1/infra/servers/{id}/backups", "infra.read", false, a.infraBackups},
		{"POST /v1/infra/servers/{id}/backups", "infra.guests.backup", true, a.infraVzdump},
		{"POST /v1/infra/servers/{id}/restore", "infra.guests.restore", true, a.infraRestore},
		{"GET /v1/infra/servers/{id}/tasks/{upid}", "infra.read", false, a.infraTask},
	}
}

func (a *API) infraRole() string {
	if a.cfg != nil && a.cfg.Roles.Infra != "" {
		return a.cfg.Roles.Infra
	}
	return "op_infra"
}

// infraRoleMessage names the configured role and the Nomen role that maps to it.
func (a *API) infraRoleMessage() string {
	role := a.infraRole()
	from := ""
	if a.cfg != nil {
		for _, m := range a.cfg.Roles.IAM {
			if m.To == role {
				from = m.From
				break
			}
		}
	}
	if from == "" {
		return "This needs the infra role " + role + "."
	}
	return "This needs the infra role " + role + ", granted in Nomen as " + from + "."
}

// infraAllowed is the configured infra role, the founder role (which passes every check), or the
// local installation owner. Platform admin alone is not implied.
func (a *API) infraAllowed(c *caller) bool {
	return c != nil && (c.founder || c.installOwner || c.op != "" && c.op == a.infraRole())
}

// infraCan is one infrastructure permission. Whoever passes infraAllowed holds them all. Anyone else needs
// a staff role that grants this one, which only counts once the team has moved to enforce mode.
func (a *API) infraCan(ctx context.Context, c *caller, perm string) bool {
	if a.infraAllowed(c) {
		return true
	}
	return c != nil && c.id != nil && a.authzMode(ctx) == "enforce" && a.staffCan(ctx, c, perm)
}

// infraGate lets a request through only with its permission. Every change is recorded before it runs,
// and a change to real infrastructure must also say why.
func (a *API) infraGate(rt infraRoute) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, ok := a.resolve(w, r)
		if !ok {
			return
		}
		if !a.infraCan(r.Context(), c, rt.perm) {
			if c != nil && c.id != nil {
				a.auditOnce(r, c, "authz.denied", rt.perm, "refused")
			}
			auth.Problem(w, http.StatusForbidden, a.infraRoleMessage()+" ("+rt.perm+")")
			return
		}
		c.infraGranted = true
		if rt.note {
			note, ok := changeNote(w, r)
			if !ok {
				return
			}
			c.note = note
		}
		if r.Method != http.MethodGet && !a.auditInfraChange(w, r, c, rt.perm) {
			return
		}
		rt.h(w, r, c)
	}
}

// noteHeader carries why a change is being made, percent-encoded so that any language survives a header.
const noteHeader = "X-Change-Note"

// changeNote reads and checks the reason for a change.
func changeNote(w http.ResponseWriter, r *http.Request) (string, bool) {
	note, err := url.PathUnescape(r.Header.Get(noteHeader))
	note = strings.TrimSpace(note)
	n := utf8.RuneCountInString(note)
	if err != nil || n < noteMin || n > noteMax || strings.IndexFunc(note, unicode.IsControl) >= 0 {
		writeHVProblem(w, http.StatusBadRequest, "note_required", fmt.Sprintf("Say why you are making this change, in %d to %d characters. It goes in the audit log.", noteMin, noteMax), "", "")
		return "", false
	}
	return note, true
}

const (
	noteMin = 5
	noteMax = 300
)

// auditInfraChange records a change against the system tenant, before it runs. With no record there is
// no change.
func (a *API) auditInfraChange(w http.ResponseWriter, r *http.Request, c *caller, perm string) bool {
	if a.db == nil && a.staffDB == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The change couldn't be audited, so it wasn't made.")
		return false
	}
	detail := perm + " · " + r.Method + " " + r.URL.Path
	if c.note != "" {
		detail += " · " + c.note
	}
	return a.auditFirst(w, r, db.AuditEntry{TenantID: servers.SystemTenantID, Actor: c.id.Sub, ActorName: displayName(c), Action: "infra.change", Detail: detail, IP: clientIP(r)})
}

func (a *API) hvDir() hvDirectory {
	if a.hvServers != nil {
		return a.hvServers
	}
	if a.db == nil {
		return emptyHV{}
	}
	return a.serverStore()
}

type emptyHV struct{}

func (emptyHV) Endpoints(context.Context) ([]servers.Endpoint, error) {
	return []servers.Endpoint{}, nil
}
func (emptyHV) Endpoint(context.Context, string) (servers.Endpoint, error) {
	return servers.Endpoint{}, servers.ErrNotFound
}
func (emptyHV) SetEndpoint(context.Context, servers.Endpoint) error { return servers.ErrNotFound }
func (emptyHV) CreateEndpoint(context.Context, servers.Endpoint, string) (servers.Endpoint, error) {
	return servers.Endpoint{}, servers.ErrNotFound
}
func (emptyHV) Registry(context.Context) ([]servers.Ref, error) { return []servers.Ref{}, nil }
func (emptyHV) StartOperation(context.Context, servers.Operation) (string, error) {
	return "", servers.ErrNotFound
}
func (emptyHV) RunOperation(context.Context, string) error { return servers.ErrNotFound }
func (emptyHV) FinishOperation(context.Context, string, string, string, string) error {
	return servers.ErrNotFound
}
func (emptyHV) RecentOutcomes(context.Context, string, time.Duration) (map[string]string, error) {
	return nil, nil
}
func (emptyHV) Operations(context.Context, string, int) ([]servers.Operation, error) {
	return []servers.Operation{}, nil
}
func (emptyHV) AbandonOperations(context.Context) (int64, error) { return 0, nil }
func (emptyHV) Fleets(context.Context) ([]servers.Fleet, error)  { return []servers.Fleet{}, nil }
func (emptyHV) SaveFleet(context.Context, servers.Fleet, string) (servers.Fleet, error) {
	return servers.Fleet{}, servers.ErrNotFound
}
func (emptyHV) DeleteFleet(context.Context, string) error      { return servers.ErrNotFound }
func (emptyHV) SetFleet(context.Context, string, string) error { return servers.ErrNotFound }

func (a *API) pve(ctx context.Context, id string) (*proxmox.Client, error) {
	ep, err := a.hvDir().Endpoint(ctx, id)
	if err != nil {
		return nil, err
	}
	return a.openEndpoint(ep)
}

// driver opens the attached hypervisor for the calls every driver shares. Calls only one driver
// has go through pve, which answers ErrUnsupported for the others.
func (a *API) driver(ctx context.Context, id string) (hv.Driver, error) {
	ep, err := a.hvDir().Endpoint(ctx, id)
	if err != nil {
		return nil, err
	}
	return a.openDriver(ep)
}

func (a *API) openDriver(ep servers.Endpoint) (hv.Driver, error) {
	switch ep.Kind {
	case "proxmox":
		c, err := a.openEndpoint(ep)
		if err != nil {
			return nil, err
		}
		return c, nil
	case "incus":
		c, err := a.openIncus(ep)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
	return nil, fmt.Errorf("%w: %q", hv.ErrUnsupported, ep.Kind)
}

func (a *API) openIncus(ep servers.Endpoint) (*incus.Client, error) {
	var ca []byte
	if ep.CAFile != "" {
		b, err := os.ReadFile(ep.CAFile)
		if err != nil {
			return nil, fmt.Errorf("incus: reading CA file: %w", err)
		}
		ca = b
	}
	cert, err := a.clientCertificate(ep)
	if err != nil {
		return nil, err
	}
	return incus.New(incus.Endpoint{BaseURL: ep.BaseURL, Project: ep.Node, CAPEM: ca, TrustedPEM: []byte(ep.TrustedCert), ClientCert: cert}, a.secretFor(ep))
}

func (a *API) openEndpoint(ep servers.Endpoint) (*proxmox.Client, error) {
	if ep.Kind != "proxmox" {
		return nil, fmt.Errorf("%w: %q", hv.ErrUnsupported, ep.Kind)
	}
	if a.openPVE != nil {
		return a.openPVE(ep)
	}
	return a.openProxmox(ep)
}

func (a *API) openProxmox(ep servers.Endpoint) (*proxmox.Client, error) {
	var ca []byte
	if ep.CAFile != "" {
		b, err := os.ReadFile(ep.CAFile)
		if err != nil {
			return nil, fmt.Errorf("proxmox: reading CA file: %w", err)
		}
		ca = b
	}
	return proxmox.New(proxmox.Endpoint{BaseURL: ep.BaseURL, Node: ep.Node, TokenID: ep.TokenID, CAPEM: ca, TrustedPEM: []byte(ep.TrustedCert)}, a.secretFor(ep))
}

func (a *API) infraServers(w http.ResponseWriter, r *http.Request, _ *caller) {
	items, err := a.hvDir().Endpoints(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Servers are unavailable right now.")
		return
	}
	if items == nil {
		items = []servers.Endpoint{}
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) infraRegistry(w http.ResponseWriter, r *http.Request, _ *caller) {
	items, err := a.hvDir().Registry(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Servers are unavailable right now.")
		return
	}
	if items == nil {
		items = []servers.Ref{}
	}
	writeJSON(w, map[string]any{"items": items})
}

type hvAttach struct {
	Kind     string `json:"kind"`
	BaseURL  string `json:"base_url"`
	Node     string `json:"node"`
	TokenID  string `json:"token_id"`
	TokenEnv string `json:"token_env"`
	CAFile   string `json:"ca_file"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
	// TokenSecret and TrustToken are write-only. They end at prepareEndpoint.
	TokenSecret string `json:"token_secret"`
	TrustToken  string `json:"trust_token"`
	TrustedCert string `json:"trusted_cert"`
}

func (in hvAttach) endpoint() servers.Endpoint {
	return servers.Endpoint{Kind: in.Kind, BaseURL: in.BaseURL, Node: in.Node, TokenID: in.TokenID, TokenEnv: in.TokenEnv, CAFile: in.CAFile, CertFile: in.CertFile, KeyFile: in.KeyFile, TrustedCert: in.TrustedCert}
}

func (a *API) infraAttach(w http.ResponseWriter, r *http.Request, c *caller) {
	var in hvAttach
	if !decodeStrict(w, r, &in) {
		return
	}
	ep := in.endpoint()
	ep.ID = r.PathValue("id")
	// Re-attaching keeps what was stored unless this request replaces it.
	if old, err := a.hvDir().Endpoint(r.Context(), ep.ID); err == nil && old.Kind == ep.Kind {
		if in.TokenSecret == "" {
			ep.SealedSecret = old.SealedSecret
		}
		if in.TrustToken == "" {
			ep.ClientCert, ep.SealedKey = old.ClientCert, old.SealedKey
			if ep.BaseURL == "" {
				ep.BaseURL = old.BaseURL
			}
			if ep.TrustedCert == "" {
				ep.TrustedCert = old.TrustedCert
			}
		}
	}
	if err := validateEndpoint(ep, in); err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.permitAttach(w, r, c, ep.ID) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), connectTimeout)
	defer cancel()
	if err := a.prepareEndpoint(ctx, &ep, in); err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.hvDir().SetEndpoint(r.Context(), ep); err != nil {
		infraErr(w, err)
		return
	}
	ep.HasSecret = len(ep.SealedKey) > 0 || len(ep.SealedSecret) > 0
	writeJSON(w, ep)
}

func (a *API) infraHealth(w http.ResponseWriter, r *http.Request, _ *caller) {
	c, err := a.driver(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	h, err := c.Health(r.Context())
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, h)
}

func (a *API) infraGuests(w http.ResponseWriter, r *http.Request, _ *caller) {
	c, err := a.driver(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	items, err := c.Guests(r.Context())
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": markSelf(a.withRecentOutcomes(r.Context(), r.PathValue("id"), items))})
}

// withRecentOutcomes shows what a change just did while the hypervisor's inventory catches up. Without
// it a guest the panel just stopped reads as running until the next inventory refresh.
func (a *API) withRecentOutcomes(ctx context.Context, server string, items []hv.Guest) []hv.Guest {
	recent, err := a.hvDir().RecentOutcomes(ctx, server, inventoryLag)
	if err != nil {
		log.Printf("infra recent outcomes %s: %v", server, err)
		return items
	}
	for i := range items {
		if status, ok := recent[items[i].ID]; ok {
			items[i].Status = status
		}
	}
	return items
}

func (a *API) infraGuest(w http.ResponseWriter, r *http.Request, _ *caller) {
	c, err := a.driver(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := c.Guest(r.Context(), r.PathValue("guest"))
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

type guestBody struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Memory  string `json:"memory"`
	Cores   string `json:"cores"`
	Disk    string `json:"disk"`
	Bridge  string `json:"bridge"`
	NewID   string `json:"new_id"`
	Target  string `json:"target"`
	Online  bool   `json:"online"`
	Storage string `json:"storage"`
	Mode    string `json:"mode"`
	Archive string `json:"archive"`
	URL     string `json:"url"`
	Content string `json:"content"`
	File    string `json:"filename"`
	DiskID  string `json:"disk_id"`
	Size    string `json:"size"`
	Snap    string `json:"snapname"`
}

func (b guestBody) spec() proxmox.Spec {
	return proxmox.Spec{Kind: b.Kind, ID: b.ID, Name: b.Name, Memory: b.Memory, Cores: b.Cores, Disk: b.Disk, Bridge: b.Bridge, NewID: b.NewID, Target: b.Target, Online: b.Online, Storage: b.Storage, Mode: b.Mode, Archive: b.Archive, URL: b.URL, Content: b.Content, File: b.File}
}

// guestManager opens a driver that can make and remove guests.
func (a *API) guestManager(ctx context.Context, id string) (hv.GuestManager, error) {
	d, err := a.driver(ctx, id)
	if err != nil {
		return nil, err
	}
	m, ok := d.(hv.GuestManager)
	if !ok {
		return nil, hv.ErrUnsupported
	}
	return m, nil
}

type newGuest struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	Storage string `json:"storage"`
	Network string `json:"network"`
	Memory  int    `json:"memory"`
	Cores   int    `json:"cores"`
	Disk    int    `json:"disk"`
	Start   bool   `json:"start"`
}

func validateGuestPlacement(in newGuest, fields []string) error {
	provided := map[string]string{"storage": in.Storage, "network": in.Network}
	allowed := map[string]bool{}
	for _, field := range fields {
		allowed[field] = true
	}
	for name, value := range provided {
		if allowed[name] && strings.TrimSpace(value) == "" {
			return fmt.Errorf("choose a %s for this guest", name)
		}
		if !allowed[name] && value != "" {
			return fmt.Errorf("%s does not apply to this provider", name)
		}
	}
	if allowed["disk"] && in.Disk == 0 {
		return errors.New("choose a positive disk size in GiB")
	}
	return nil
}

func (a *API) infraCreate(w http.ResponseWriter, r *http.Request, c *caller) {
	var in newGuest
	if !decodeStrict(w, r, &in) {
		return
	}
	if in.Name == "" || (in.Kind != hv.KindCT && in.Kind != hv.KindVM) || in.Memory < 0 || in.Cores < 0 || in.Disk < 0 {
		auth.Problem(w, http.StatusBadRequest, "A new guest needs a name and a kind, and any sizes must be whole numbers.")
		return
	}
	ep, err := a.hvDir().Endpoint(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	spec, known := hv.SpecFor(ep.Kind)
	if !known {
		auth.Problem(w, http.StatusBadRequest, "The attached provider has no guest creation contract.")
		return
	}
	if err := validateGuestPlacement(in, spec.CreateFields); err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.permit(w, r, c, r.PathValue("id"), in.Name, "create") {
		return
	}
	m, err := a.guestManager(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := a.tracked(r, c, r.PathValue("id"), in.Name, "create", func(ctx context.Context) (hv.Guest, error) {
		return m.CreateGuest(ctx, hv.GuestSpec{Kind: in.Kind, Name: in.Name, Image: in.Image, Storage: in.Storage, Network: in.Network, Memory: in.Memory, Cores: in.Cores, Disk: in.Disk, Start: in.Start})
	})
	if err != nil {
		infraErr(w, err)
		return
	}
	writeStatus(w, http.StatusCreated, g)
}

// infraImages is what a guest can be made from: what the host has, then what this installation names.
func (a *API) infraImages(w http.ResponseWriter, r *http.Request, _ *caller) {
	id := r.PathValue("id")
	ep, err := a.hvDir().Endpoint(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	m, err := a.guestManager(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	items, err := m.Images(r.Context())
	if err != nil {
		infraErr(w, err)
		return
	}
	cfg, _ := a.uiConfig(r.Context()).Provider(ep.Kind)
	for _, i := range cfg.Images {
		items = append(items, hv.Image{ID: i.ID, Label: i.Label, Kind: i.Kind})
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) infraClone(w http.ResponseWriter, r *http.Request, c *caller) {
	var in guestBody
	if !decodeStrict(w, r, &in) {
		return
	}
	if !a.permit(w, r, c, r.PathValue("id"), r.PathValue("guest"), "clone") {
		return
	}
	cl, err := a.pve(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := a.tracked(r, c, r.PathValue("id"), r.PathValue("guest"), "clone", func(ctx context.Context) (hv.Guest, error) {
		return cl.Clone(ctx, r.PathValue("guest"), in.spec())
	})
	if err != nil {
		infraErr(w, err)
		return
	}
	writeStatus(w, http.StatusCreated, g)
}

func (a *API) infraTemplate(w http.ResponseWriter, r *http.Request, c *caller) {
	id, guest := r.PathValue("id"), r.PathValue("guest")
	cl, err := a.pve(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	if a.refuseSelf(w, r, c, cl, guest, "template") {
		return
	}
	if !a.permit(w, r, c, id, guest, "template") {
		return
	}
	g, err := a.tracked(r, c, id, guest, "template", func(ctx context.Context) (hv.Guest, error) { return cl.Template(ctx, guest) })
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

func (a *API) infraDelete(w http.ResponseWriter, r *http.Request, c *caller) {
	id, guest := r.PathValue("id"), r.PathValue("guest")
	d, err := a.driver(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	if a.refuseSelf(w, r, c, d, guest, "delete") {
		return
	}
	if !a.permit(w, r, c, id, guest, "delete") {
		return
	}
	cl, err := a.guestManager(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	if _, err := a.tracked(r, c, id, guest, "delete", func(ctx context.Context) (hv.Guest, error) { return hv.Guest{}, cl.DeleteGuest(ctx, guest) }); err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, map[string]bool{"deleted": true})
}

func (a *API) infraConfig(w http.ResponseWriter, r *http.Request, c *caller) {
	var in map[string]string
	if !decodeStrict(w, r, &in) {
		return
	}
	id, guest := r.PathValue("id"), r.PathValue("guest")
	if !a.permit(w, r, c, id, guest, "config") {
		return
	}
	cl, err := a.pve(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	cfg, err := cl.SetConfig(r.Context(), guest, in)
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, cfg)
}

func (a *API) infraResize(w http.ResponseWriter, r *http.Request, c *caller) {
	var in guestBody
	if !decodeStrict(w, r, &in) {
		return
	}
	id, guest := r.PathValue("id"), r.PathValue("guest")
	if !a.permit(w, r, c, id, guest, "resize") {
		return
	}
	cl, err := a.pve(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := a.tracked(r, c, id, guest, "resize", func(ctx context.Context) (hv.Guest, error) { return cl.Resize(ctx, guest, in.DiskID, in.Size) })
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

func (a *API) infraMigrate(w http.ResponseWriter, r *http.Request, c *caller) {
	var in guestBody
	if !decodeStrict(w, r, &in) {
		return
	}
	id, guest := r.PathValue("id"), r.PathValue("guest")
	cl, err := a.pve(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	if a.refuseSelf(w, r, c, cl, guest, "migrate") {
		return
	}
	if !a.permit(w, r, c, id, guest, "migrate") {
		return
	}
	g, err := a.tracked(r, c, id, guest, "migrate", func(ctx context.Context) (hv.Guest, error) { return cl.Migrate(ctx, guest, in.spec()) })
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

func (a *API) infraStatus(w http.ResponseWriter, r *http.Request, c *caller) {
	verb := r.PathValue("verb")
	switch verb {
	case "start", "stop", "shutdown", "reboot":
	default:
		auth.Problem(w, http.StatusBadRequest, "Verb must be start, stop, shutdown, or reboot.")
		return
	}
	id, guest := r.PathValue("id"), r.PathValue("guest")
	cl, err := a.driver(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	if a.refuseSelf(w, r, c, cl, guest, verb) {
		return
	}
	if !a.permit(w, r, c, id, guest, verb) {
		return
	}
	g, err := a.tracked(r, c, id, guest, verb, func(ctx context.Context) (hv.Guest, error) { return runVerb(ctx, cl, verb, guest) })
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

// snapshotKeeper opens a driver that can list and restore its snapshots.
func (a *API) snapshotKeeper(ctx context.Context, id string) (hv.SnapshotKeeper, error) {
	d, err := a.driver(ctx, id)
	if err != nil {
		return nil, err
	}
	keeper, ok := d.(hv.SnapshotKeeper)
	if !ok {
		return nil, hv.ErrUnsupported
	}
	return keeper, nil
}

func (a *API) infraSnapshots(w http.ResponseWriter, r *http.Request, _ *caller) {
	cl, err := a.snapshotKeeper(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	names, err := cl.Snapshots(r.Context(), r.PathValue("guest"))
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": names})
}

func (a *API) infraSnapshot(w http.ResponseWriter, r *http.Request, c *caller) {
	var in guestBody
	if !decodeStrict(w, r, &in) {
		return
	}
	id, guest := r.PathValue("id"), r.PathValue("guest")
	if !a.permit(w, r, c, id, guest, "snapshot") {
		return
	}
	cl, err := a.driver(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := a.tracked(r, c, id, guest, "snapshot", func(ctx context.Context) (hv.Guest, error) { return cl.Snapshot(ctx, guest, in.Snap) })
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

func (a *API) infraRollback(w http.ResponseWriter, r *http.Request, c *caller) {
	id, guest, name := r.PathValue("id"), r.PathValue("guest"), r.PathValue("name")
	d, err := a.driver(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	if a.refuseSelf(w, r, c, d, guest, "rollback") {
		return
	}
	if !a.permit(w, r, c, id, guest, "rollback") {
		return
	}
	cl, err := a.snapshotKeeper(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := a.tracked(r, c, id, guest, "rollback", func(ctx context.Context) (hv.Guest, error) { return cl.Rollback(ctx, guest, name) })
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

func (a *API) infraStorage(w http.ResponseWriter, r *http.Request, _ *caller) {
	cl, err := a.pve(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	items, err := cl.Storage(r.Context())
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) infraContent(w http.ResponseWriter, r *http.Request, _ *caller) {
	cl, err := a.pve(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	items, err := cl.Content(r.Context(), r.PathValue("storage"), r.URL.Query().Get("content"))
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) infraDownload(w http.ResponseWriter, r *http.Request, c *caller) {
	var in guestBody
	if !decodeStrict(w, r, &in) {
		return
	}
	in.Storage = r.PathValue("storage")
	id := r.PathValue("id")
	if !a.permit(w, r, c, id, in.Storage, "download") {
		return
	}
	cl, err := a.pve(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	upid, err := cl.DownloadURL(r.Context(), in.spec())
	if err != nil {
		infraErr(w, err)
		return
	}
	writeStatus(w, http.StatusAccepted, map[string]string{"upid": upid})
}

func (a *API) infraUpload(w http.ResponseWriter, r *http.Request, c *caller) {
	id, storage := r.PathValue("id"), r.PathValue("storage")
	if !a.permit(w, r, c, id, storage, "upload") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Upload must be multipart and at most 32 MiB. Larger images use download-url.")
		return
	}
	f, hdr, err := r.FormFile("filename")
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, "Attach the file as filename.")
		return
	}
	defer f.Close()
	cl, err := a.pve(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	upid, err := cl.Upload(r.Context(), storage, r.FormValue("content"), hdr.Filename, f)
	if err != nil {
		infraErr(w, err)
		return
	}
	writeStatus(w, http.StatusAccepted, map[string]string{"upid": upid})
}

func (a *API) infraServices(w http.ResponseWriter, r *http.Request, _ *caller) {
	cl, err := a.pve(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	items, err := cl.Services(r.Context())
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) infraNetwork(w http.ResponseWriter, r *http.Request, _ *caller) {
	cl, err := a.pve(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	items, err := cl.NodeNetworks(r.Context())
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) infraFirewall(w http.ResponseWriter, r *http.Request, _ *caller) {
	cl, err := a.pve(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	items, err := cl.FirewallRules(r.Context(), r.URL.Query().Get("guest"))
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) infraBackups(w http.ResponseWriter, r *http.Request, _ *caller) {
	storage := r.URL.Query().Get("storage")
	if storage == "" {
		auth.Problem(w, http.StatusBadRequest, "Pass storage.")
		return
	}
	cl, err := a.pve(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	items, err := cl.Backups(r.Context(), storage)
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) infraVzdump(w http.ResponseWriter, r *http.Request, c *caller) {
	var in guestBody
	if !decodeStrict(w, r, &in) {
		return
	}
	if !a.permit(w, r, c, r.PathValue("id"), in.ID, "backup") {
		return
	}
	cl, err := a.pve(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	upid, err := cl.Vzdump(r.Context(), in.spec())
	if err != nil {
		infraErr(w, err)
		return
	}
	writeStatus(w, http.StatusAccepted, map[string]string{"upid": upid})
}

func (a *API) infraRestore(w http.ResponseWriter, r *http.Request, c *caller) {
	var in guestBody
	if !decodeStrict(w, r, &in) {
		return
	}
	if !a.permit(w, r, c, r.PathValue("id"), in.ID, "restore") {
		return
	}
	cl, err := a.pve(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := a.tracked(r, c, r.PathValue("id"), in.ID, "restore", func(ctx context.Context) (hv.Guest, error) { return cl.Restore(ctx, in.spec()) })
	if err != nil {
		infraErr(w, err)
		return
	}
	writeStatus(w, http.StatusCreated, g)
}

func (a *API) infraTask(w http.ResponseWriter, r *http.Request, _ *caller) {
	cl, err := a.pve(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	t, err := cl.Task(r.Context(), r.PathValue("upid"))
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, t)
}

// permit checks the approval policy, files the board card when the rule asks for one,
// and writes an audit entry that names the verb and ids only.
// An approved card is the whole gate. The caller retries the same request.
func (a *API) permit(w http.ResponseWriter, r *http.Request, c *caller, server, guest, verb string) bool {
	return a.permitFor(w, r, c, "", server, guest, verb)
}

// permitAttach gates adding or detaching an API. Sites whose policy predates hv.attach keep
// their lifecycle rule for it.
func (a *API) permitAttach(w http.ResponseWriter, r *http.Request, c *caller, server string) bool {
	action := hv.ActionAttach
	if !a.cfg.Approval.HasAction(action) {
		action = hv.ActionLifecycle
	}
	return a.permitAction(w, r, c, action, "", server, server, "endpoint")
}

func (a *API) permitFor(w http.ResponseWriter, r *http.Request, c *caller, tenant, server, guest, verb string) bool {
	return a.permitAction(w, r, c, hv.ActionLifecycle, tenant, server, guest, verb)
}

func (a *API) permitAction(w http.ResponseWriter, r *http.Request, c *caller, action, tenant, server, guest, verb string) bool {
	if !a.cfg.Approval.Allows(hvSubject(a, c, tenant), action, server) {
		writeHVProblem(w, http.StatusForbidden, "approval_denied", "The approval policy does not allow this action.", "", "")
		return false
	}
	key := fmt.Sprintf("%s:%s:%s:%s", action, server, guest, verb)
	if a.hvNeedsBoard(r.Context(), action, server) {
		org := a.boardOrg(c, tenant)
		if org == "" {
			writeHVProblem(w, http.StatusConflict, "board_org_missing", "Set approval.board.org_id so the console can file the approval card.", key, "")
			return false
		}
		cards := a.lifecycleBoard()
		if cards == nil {
			writeHVProblem(w, http.StatusConflict, "waiting_on_board", "The approval board is not configured.", key, "")
			return false
		}
		body := "Approve this card, then run the change again. An approval covers one change."
		if c.note != "" {
			body = "Why: " + c.note + "\n\n" + body
		}
		st, err := cards.Post(r.Context(), org, key, changeTitle(verb, guest), body, c.id.Sub)
		if err != nil {
			infraErr(w, err)
			return false
		}
		if st != "approved" {
			writeBoardWait(r.Context(), w, cards, org, key, verb, guest, st, a.cfg.Approval.Allows(approverSubject(a, c), "hub.decide", ""))
			return false
		}
		// An approval covers one change. Only one request can take it.
		if user, ok := cards.(cardUser); ok {
			used, err := user.UseCard(r.Context(), org, key)
			if err != nil {
				infraErr(w, err)
				return false
			}
			if !used {
				writeBoardWait(r.Context(), w, cards, org, key, verb, guest, st, a.cfg.Approval.Allows(approverSubject(a, c), "hub.decide", ""))
				return false
			}
		}
	}
	if a.db == nil && a.staffDB == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The change couldn't be audited, so it wasn't made.")
		return false
	}
	detail := server + " " + verb + " " + guest
	if c.note != "" {
		detail += " · " + c.note
	}
	return a.auditFirst(w, r, db.AuditEntry{Actor: c.id.Sub, ActorName: displayName(c), Action: "hv.lifecycle", Detail: detail, IP: clientIP(r)})
}

func hvSubject(a *API, c *caller, tenant string) approval.Subject {
	sub := approverSubject(a, c)
	// The gate already found this permission in a staff role, which the policy cannot see.
	if c != nil && c.infraGranted && tenant == "" {
		sub.Roles = append(sub.Roles, a.infraRole())
	}
	if c == nil || tenant == "" {
		return sub
	}
	for i := range c.tenants {
		if c.tenants[i].ID == tenant {
			sub.Roles = append(sub.Roles, c.tenants[i].Roles...)
		}
	}
	return sub
}

func (a *API) boardOrg(c *caller, tenant string) string {
	if org := a.cfg.Approval.Board.OrgID; org != "" {
		return org
	}
	if tenant != "" {
		return tenant
	}
	if c != nil && c.id != nil && c.id.OrgID != "" {
		return c.id.OrgID
	}
	return a.cfg.Operator.PlatformOrgID
}

// cardBoard files an approval card and reads it back. Status is the board word (approved, needs_approval, …).
type cardBoard interface {
	Status(ctx context.Context, org, key string) (string, error)
	Post(ctx context.Context, org, key, title, body, by string) (string, error)
}

// cardUser is a board that can spend an approval, so that it covers one change and not all later ones.
type cardUser interface {
	UseCard(ctx context.Context, org, key string) (bool, error)
}

// cardFinder is a board on this panel, whose cards can be decided from the page that asked.
type cardFinder interface {
	CardID(ctx context.Context, org, key string) (string, error)
}

// changeTitle names a change for people. A change to many guests has no one guest to name.
func changeTitle(verb, guest string) string {
	if guest == "*" {
		return strings.ReplaceAll(verb, "-", " ")
	}
	return verb + " " + guest
}

// writeBoardWait says a change is waiting. The card's id goes only to someone who can decide it, which is
// what offers them the decision on the page.
func writeBoardWait(ctx context.Context, w http.ResponseWriter, cards cardBoard, org, key, verb, guest, st string, canDecide bool) {
	id := ""
	if finder, ok := cards.(cardFinder); ok && canDecide {
		if found, err := finder.CardID(ctx, org, key); err == nil {
			id = found
		}
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "about:blank", "title": http.StatusText(http.StatusConflict), "status": http.StatusConflict, "code": "waiting_on_board",
		"message": changeTitle(verb, guest) + " needs approval before it runs.", "card_key": key, "board_status": st, "card_id": id,
	})
}

func (a *API) lifecycleBoard() cardBoard {
	if a.hvBoard != nil {
		return a.hvBoard
	}
	b := a.cfg.Approval.Board
	switch b.Driver {
	case "", "off":
		return nil
	case "http":
		token := ""
		if b.TokenEnv != "" {
			token = os.Getenv(b.TokenEnv)
		}
		return decisionBoard{src: approval.HTTPBoard{BaseURL: b.BaseURL, Token: token, OrgID: b.OrgID}}
	case "hub":
		if a.db == nil {
			return missingBoard{}
		}
		return decisionBoard{src: hubBoard{store: &hub.Store{Pool: a.db.Pool}, org: b.OrgID}}
	default:
		return missingBoard{}
	}
}

type missingBoard struct{}

func (missingBoard) Status(context.Context, string, string) (string, error) {
	return "", fmt.Errorf("approval board is not configured")
}

func (missingBoard) Post(context.Context, string, string, string, string, string) (string, error) {
	return "", fmt.Errorf("approval board is not configured")
}

type decisionSource interface {
	Status(ctx context.Context, org, key string) (approval.Decision, error)
	Post(ctx context.Context, req approval.Request) (approval.Decision, error)
}

type decisionBoard struct{ src decisionSource }

func (d decisionBoard) Status(ctx context.Context, org, key string) (string, error) {
	dec, err := d.src.Status(ctx, org, key)
	if err != nil {
		return "", err
	}
	return dec.Status, nil
}

func (d decisionBoard) Post(ctx context.Context, org, key, title, body, by string) (string, error) {
	dec, err := d.src.Post(ctx, approval.Request{Key: key, OrgID: org, Action: hv.ActionLifecycle, Title: title, Body: body, By: by})
	if err != nil {
		return "", err
	}
	return dec.Status, nil
}

// UseCard spends the approval when the source can. A board elsewhere keeps its own rule.
func (d decisionBoard) UseCard(ctx context.Context, org, key string) (bool, error) {
	if user, ok := d.src.(cardUser); ok {
		return user.UseCard(ctx, org, key)
	}
	return true, nil
}

// CardID finds the card when it lives on this panel. A board elsewhere has none to offer.
func (d decisionBoard) CardID(ctx context.Context, org, key string) (string, error) {
	if finder, ok := d.src.(cardFinder); ok {
		return finder.CardID(ctx, org, key)
	}
	return "", nil
}

func writeHVProblem(w http.ResponseWriter, status int, code, msg, key, board string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "about:blank", "title": http.StatusText(status), "status": status,
		"code": code, "message": msg, "card_key": key, "board_status": board,
	})
}

var nodeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,62}$`)
var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// validateEndpoint checks what the driver of e.Kind needs. Nothing here dials the address.
func validateEndpoint(e servers.Endpoint, in hvAttach) error {
	spec, ok := hv.SpecFor(e.Kind)
	if !ok {
		kinds := []string{}
		for _, s := range hv.Specs() {
			kinds = append(kinds, s.Kind)
		}
		return fmt.Errorf("kind must be one of %s", strings.Join(kinds, ", "))
	}
	values := map[string]string{hv.FieldBaseURL: e.BaseURL, hv.FieldNode: e.Node, hv.FieldTokenID: e.TokenID, hv.FieldTokenEnv: e.TokenEnv,
		hv.FieldCAFile: e.CAFile, hv.FieldCertFile: e.CertFile, hv.FieldKeyFile: e.KeyFile, hv.FieldTokenSecret: in.TokenSecret, hv.FieldTrustToken: in.TrustToken}
	allowed := map[string]bool{}
	for _, f := range spec.Fields {
		allowed[f.Name] = true
		if f.Required && values[f.Name] == "" {
			return fmt.Errorf("%s is required for %s", f.Name, e.Kind)
		}
	}
	for name, v := range values {
		if v != "" && !allowed[name] {
			return fmt.Errorf("%s does not apply to %s", name, e.Kind)
		}
	}
	// A trust token says where the server is; a typed address is checked either way.
	if in.TrustToken == "" || e.BaseURL != "" {
		u, err := url.Parse(e.BaseURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
			return fmt.Errorf("base_url must be https with a host and no user, query, or path")
		}
	}
	if in.TrustedCert != "" {
		if _, err := hv.ParseCertificate([]byte(in.TrustedCert)); err != nil {
			return fmt.Errorf("trusted_cert is not a certificate")
		}
	}
	if e.Node != "" && !nodeName.MatchString(e.Node) {
		return fmt.Errorf("node must be a hostname label")
	}
	if e.TokenID != "" && (strings.ContainsAny(e.TokenID, " \t\r\n") || strings.Contains(e.TokenID, "://")) {
		return fmt.Errorf("token_id must be the API token id, not a secret or URL")
	}
	if e.TokenEnv != "" && !envName.MatchString(e.TokenEnv) {
		return fmt.Errorf("token_env must name an environment variable")
	}
	for name, p := range map[string]string{hv.FieldCAFile: e.CAFile, hv.FieldCertFile: e.CertFile, hv.FieldKeyFile: e.KeyFile} {
		if p != "" && (strings.Contains(p, "..") || strings.Contains(p, "://")) {
			return fmt.Errorf("%s must be a PEM path", name)
		}
	}
	if (e.CertFile == "") != (e.KeyFile == "") {
		return fmt.Errorf("cert_file and key_file go together")
	}
	if e.Kind == "incus" && in.TrustToken == "" && e.CertFile == "" && e.TokenEnv == "" && e.ClientCert == "" {
		return fmt.Errorf("incus needs a trust token, a client certificate, or token_env")
	}
	if e.Kind == "proxmox" && in.TokenSecret == "" && e.TokenEnv == "" && len(e.SealedSecret) == 0 {
		return fmt.Errorf("proxmox needs the API token secret")
	}
	return nil
}

func infraErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, servers.ErrNotFound):
		auth.Problem(w, http.StatusNotFound, "No hypervisor is attached to that server.")
	case errors.Is(err, hv.ErrDenied):
		auth.Problem(w, http.StatusForbidden, "The approval policy does not allow this.")
	case errors.Is(err, hv.ErrWaiting):
		auth.Problem(w, http.StatusConflict, "This is waiting on the factory board.")
	case errors.Is(err, hv.ErrUnsupported):
		auth.Problem(w, http.StatusConflict, "This provider does not offer that action.")
	case errors.Is(err, hv.ErrGuestNotFound):
		auth.Problem(w, http.StatusNotFound, "The hypervisor has no guest with that id.")
	case strings.Contains(err.Error(), "not in inventory"):
		auth.Problem(w, http.StatusNotFound, "The hypervisor has no guest with that id.")
	default:
		auth.Problem(w, http.StatusBadGateway, err.Error())
	}
}
