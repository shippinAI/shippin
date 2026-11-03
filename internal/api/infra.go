package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

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
}

type infraRoute struct {
	pattern string
	h       func(http.ResponseWriter, *http.Request, *caller)
}

func (a *API) infraRoutes(mux Router) {
	for _, rt := range a.infraRouteTable() {
		mux.HandleFunc(rt.pattern, a.flow.Require(a.infraGate(rt.h)))
	}
	a.tenantHVRoutes(mux)
}

func (a *API) infraRouteTable() []infraRoute {
	return []infraRoute{
		{"GET /v1/infra/overview", a.infraOverview},
		{"GET /v1/infra/servers", a.infraServers},
		{"POST /v1/infra/servers", a.infraAdd},
		{"GET /v1/infra/servers/registry", a.infraRegistry},
		{"PUT /v1/infra/servers/{id}", a.infraAttach},
		{"DELETE /v1/infra/servers/{id}/attachment", a.infraDetach},
		{"GET /v1/infra/servers/{id}/health", a.infraHealth},
		{"GET /v1/infra/servers/{id}/guests", a.infraGuests},
		{"GET /v1/infra/servers/{id}/guests/{guest}", a.infraGuest},
		{"POST /v1/infra/servers/{id}/guests", a.infraCreate},
		{"POST /v1/infra/servers/{id}/guests/{guest}/clone", a.infraClone},
		{"POST /v1/infra/servers/{id}/guests/{guest}/template", a.infraTemplate},
		{"DELETE /v1/infra/servers/{id}/guests/{guest}", a.infraDelete},
		{"PUT /v1/infra/servers/{id}/guests/{guest}/config", a.infraConfig},
		{"POST /v1/infra/servers/{id}/guests/{guest}/resize", a.infraResize},
		{"POST /v1/infra/servers/{id}/guests/{guest}/migrate", a.infraMigrate},
		{"POST /v1/infra/servers/{id}/guests/{guest}/status/{verb}", a.infraStatus},
		{"GET /v1/infra/servers/{id}/guests/{guest}/snapshots", a.infraSnapshots},
		{"POST /v1/infra/servers/{id}/guests/{guest}/snapshots", a.infraSnapshot},
		{"POST /v1/infra/servers/{id}/guests/{guest}/snapshots/{name}/rollback", a.infraRollback},
		{"GET /v1/infra/servers/{id}/storage", a.infraStorage},
		{"GET /v1/infra/servers/{id}/storage/{storage}/content", a.infraContent},
		{"POST /v1/infra/servers/{id}/storage/{storage}/download-url", a.infraDownload},
		{"POST /v1/infra/servers/{id}/storage/{storage}/upload", a.infraUpload},
		{"GET /v1/infra/servers/{id}/services", a.infraServices},
		{"GET /v1/infra/servers/{id}/network", a.infraNetwork},
		{"GET /v1/infra/servers/{id}/firewall/rules", a.infraFirewall},
		{"GET /v1/infra/servers/{id}/backups", a.infraBackups},
		{"POST /v1/infra/servers/{id}/backups", a.infraVzdump},
		{"POST /v1/infra/servers/{id}/restore", a.infraRestore},
		{"GET /v1/infra/servers/{id}/tasks/{upid}", a.infraTask},
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

// infraAllowed is the configured infra role, or the local installation owner. Platform admin
// is not implied for an IdP user.
func (a *API) infraAllowed(c *caller) bool {
	return c != nil && (c.installOwner || c.op != "" && c.op == a.infraRole())
}

func (a *API) infraGate(h func(http.ResponseWriter, *http.Request, *caller)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, ok := a.resolve(w, r)
		if !ok {
			return
		}
		if !a.infraAllowed(c) {
			auth.Problem(w, http.StatusForbidden, a.infraRoleMessage())
			return
		}
		h(w, r, c)
	}
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
		c, err := openIncus(ep)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
	return nil, fmt.Errorf("%w: %q", hv.ErrUnsupported, ep.Kind)
}

func openIncus(ep servers.Endpoint) (*incus.Client, error) {
	var ca []byte
	if ep.CAFile != "" {
		b, err := os.ReadFile(ep.CAFile)
		if err != nil {
			return nil, fmt.Errorf("incus: reading CA file: %w", err)
		}
		ca = b
	}
	var cert *tls.Certificate
	if ep.CertFile != "" {
		c, err := tls.LoadX509KeyPair(ep.CertFile, ep.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("incus: reading client certificate: %w", err)
		}
		cert = &c
	}
	env := ep.TokenEnv
	return incus.New(incus.Endpoint{BaseURL: ep.BaseURL, Project: ep.Node, CAPEM: ca, ClientCert: cert}, func() (string, error) {
		if env == "" {
			return "", nil
		}
		s := os.Getenv(env)
		if s == "" {
			return "", fmt.Errorf("incus: token env %s is empty", env)
		}
		return s, nil
	})
}

func (a *API) openEndpoint(ep servers.Endpoint) (*proxmox.Client, error) {
	if ep.Kind != "proxmox" {
		return nil, fmt.Errorf("%w: %q", hv.ErrUnsupported, ep.Kind)
	}
	if a.openPVE != nil {
		return a.openPVE(ep)
	}
	return openProxmox(ep)
}

func openProxmox(ep servers.Endpoint) (*proxmox.Client, error) {
	var ca []byte
	if ep.CAFile != "" {
		b, err := os.ReadFile(ep.CAFile)
		if err != nil {
			return nil, fmt.Errorf("proxmox: reading CA file: %w", err)
		}
		ca = b
	}
	env := ep.TokenEnv
	return proxmox.New(proxmox.Endpoint{BaseURL: ep.BaseURL, Node: ep.Node, TokenID: ep.TokenID, CAPEM: ca}, func() (string, error) {
		if env == "" {
			return "", fmt.Errorf("proxmox: token env is not configured")
		}
		s := os.Getenv(env)
		if s == "" {
			return "", fmt.Errorf("proxmox: token env %s is empty", env)
		}
		return s, nil
	})
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
}

func (in hvAttach) endpoint() servers.Endpoint {
	return servers.Endpoint{Kind: in.Kind, BaseURL: in.BaseURL, Node: in.Node, TokenID: in.TokenID, TokenEnv: in.TokenEnv, CAFile: in.CAFile, CertFile: in.CertFile, KeyFile: in.KeyFile}
}

func (a *API) infraAttach(w http.ResponseWriter, r *http.Request, c *caller) {
	var in hvAttach
	if !decodeStrict(w, r, &in) {
		return
	}
	ep := in.endpoint()
	ep.ID = r.PathValue("id")
	if err := validateEndpoint(ep); err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.permitAttach(w, r, c, ep.ID) {
		return
	}
	if err := a.hvDir().SetEndpoint(r.Context(), ep); err != nil {
		infraErr(w, err)
		return
	}
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
	writeJSON(w, map[string]any{"items": items})
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

func (a *API) infraCreate(w http.ResponseWriter, r *http.Request, c *caller) {
	var in guestBody
	if !decodeStrict(w, r, &in) {
		return
	}
	if !a.permit(w, r, c, r.PathValue("id"), in.ID, "create") {
		return
	}
	cl, err := a.pve(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := cl.Create(r.Context(), in.spec())
	if err != nil {
		infraErr(w, err)
		return
	}
	writeStatus(w, http.StatusCreated, g)
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
	g, err := cl.Clone(r.Context(), r.PathValue("guest"), in.spec())
	if err != nil {
		infraErr(w, err)
		return
	}
	writeStatus(w, http.StatusCreated, g)
}

func (a *API) infraTemplate(w http.ResponseWriter, r *http.Request, c *caller) {
	id, guest := r.PathValue("id"), r.PathValue("guest")
	if !a.permit(w, r, c, id, guest, "template") {
		return
	}
	cl, err := a.pve(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := cl.Template(r.Context(), guest)
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

func (a *API) infraDelete(w http.ResponseWriter, r *http.Request, c *caller) {
	id, guest := r.PathValue("id"), r.PathValue("guest")
	if !a.permit(w, r, c, id, guest, "delete") {
		return
	}
	cl, err := a.pve(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	if err := cl.Delete(r.Context(), guest); err != nil {
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
	g, err := cl.Resize(r.Context(), guest, in.DiskID, in.Size)
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
	if !a.permit(w, r, c, id, guest, "migrate") {
		return
	}
	cl, err := a.pve(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := cl.Migrate(r.Context(), guest, in.spec())
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
	if !a.permit(w, r, c, id, guest, verb) {
		return
	}
	cl, err := a.driver(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	var g hv.Guest
	switch verb {
	case "start":
		g, err = cl.Start(r.Context(), guest)
	case "stop":
		g, err = cl.Stop(r.Context(), guest)
	case "shutdown":
		g, err = cl.Shutdown(r.Context(), guest)
	default:
		g, err = cl.Reboot(r.Context(), guest)
	}
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

func (a *API) infraSnapshots(w http.ResponseWriter, r *http.Request, _ *caller) {
	cl, err := a.pve(r.Context(), r.PathValue("id"))
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
	g, err := cl.Snapshot(r.Context(), guest, in.Snap)
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, g)
}

func (a *API) infraRollback(w http.ResponseWriter, r *http.Request, c *caller) {
	id, guest, name := r.PathValue("id"), r.PathValue("guest"), r.PathValue("name")
	if !a.permit(w, r, c, id, guest, "rollback") {
		return
	}
	cl, err := a.pve(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	g, err := cl.Rollback(r.Context(), guest, name)
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
	g, err := cl.Restore(r.Context(), in.spec())
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
		st, err := cards.Post(r.Context(), org, key, verb+" "+guest, "Approve this card, then retry the same request.", c.id.Sub)
		if err != nil {
			infraErr(w, err)
			return false
		}
		if st != "approved" {
			writeHVProblem(w, http.StatusConflict, "waiting_on_board",
				fmt.Sprintf("Approval card %s is %s. Approve it on the factory board, then retry this same call.", key, st),
				key, st)
			return false
		}
	}
	if a.db == nil && a.staffDB == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The change couldn't be audited, so it wasn't made.")
		return false
	}
	return a.auditFirst(w, r, db.AuditEntry{Actor: c.id.Sub, ActorName: displayName(c), Action: "hv.lifecycle",
		Detail: server + " " + verb + " " + guest, IP: clientIP(r)})
}

func hvSubject(a *API, c *caller, tenant string) approval.Subject {
	sub := approverSubject(a, c)
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
func validateEndpoint(e servers.Endpoint) error {
	spec, ok := hv.SpecFor(e.Kind)
	if !ok {
		kinds := []string{}
		for _, s := range hv.Specs() {
			kinds = append(kinds, s.Kind)
		}
		return fmt.Errorf("kind must be one of %s", strings.Join(kinds, ", "))
	}
	values := map[string]string{hv.FieldBaseURL: e.BaseURL, hv.FieldNode: e.Node, hv.FieldTokenID: e.TokenID,
		hv.FieldTokenEnv: e.TokenEnv, hv.FieldCAFile: e.CAFile, hv.FieldCertFile: e.CertFile, hv.FieldKeyFile: e.KeyFile}
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
	u, err := url.Parse(e.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return fmt.Errorf("base_url must be https with a host and no user, query, or path")
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
	if e.Kind == "incus" && e.CertFile == "" && e.TokenEnv == "" {
		return fmt.Errorf("incus needs a client certificate or token_env")
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
	case strings.Contains(err.Error(), "not in inventory"):
		auth.Problem(w, http.StatusNotFound, "The hypervisor has no guest with that id.")
	default:
		auth.Problem(w, http.StatusBadGateway, err.Error())
	}
}
