// Package api serves the control-plane endpoints used by the console.
package api

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"shippin.cloud/console/internal/agentruns"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/deployments"
	"shippin.cloud/console/internal/gh"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/hv/proxmox"
	"shippin.cloud/console/internal/identity"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/mail"
	"shippin.cloud/console/internal/managed"
	"shippin.cloud/console/internal/servers"
	"shippin.cloud/console/internal/subs"
	"shippin.cloud/console/internal/tenantdb"
)

type API struct {
	cfg          *config.Config
	flow         *auth.Flow
	db           *db.DB
	host         *hostclient.Client
	updates      updateChecker
	started      time.Time
	probeGate    *probeConcurrencyGate
	probeLimitMu sync.Mutex
	// seen dedupes per-request grant access entries from forward_auth.
	seen seenCache
	// resolveCaller overrides the IdP caller lookup in tests only.
	resolveCaller func(http.ResponseWriter, *http.Request) (*caller, bool)
	// notify queues e-mail notices; nil when mail is disabled.
	notify *mail.Notifier
	// directory is the local account service. Nil when the process has not wired it.
	directory *identity.Service
	// Test-only overrides: recipient lookups and synchronous notices.
	lookupManagers func(ctx context.Context, org string) ([]string, error)
	orgOfUser      func(ctx context.Context, sub string) (string, error) // test seam for orgMember
	lookupUser     func(ctx context.Context, sub string) (string, error)
	notifySync     bool
	// Staff authorization: store/IdP overrides (tests), resolved-role cache, settings cache.
	staffDB    staffStore
	staffIdP   staffIdP
	tenantIdP  tenantIdP      // tenant Team page (Nomen org members); nil = the live IdP
	permDB     permGrantStore // direct permission grants; nil = a.db (tests: fake or none)
	staffCache staffCache
	// authAt overrides the sign-in time lookup in tests only.
	authAt func(*http.Request) time.Time
	// jobs is the durable job store; runner is nil while config jobs.enabled is false.
	jobs        *jobs.Store
	runner      *jobs.Runner
	drift       driftMemo
	subs        *subs.Store // M2: per-user AI homes and subscription status (never credentials)
	gh          *gh.App     // M3: the Shippin GitHub App; nil when github is disabled
	ghTick      ghTimers
	runs        *agentruns.Store   // M4: build-agent runs (no tokens, prompt only until delivered)
	svcs        *managed.Store     // managed service instances (rrflow/state first)
	deployments *deployments.Store // durable desired state; provider execution is separately gated
	agents      agentDeps          // N2 agent identities (IdP service users)
	n2          n2Deps             // N2 OpenFGA shadow checks and the Vaultix client
	// Tenant starter-database console. Nil stores fall back to a.db.
	tenantDB *tenantdb.Gate
	sqlLog   sqlLogger
	snippets snippetStore
	backups  tenantdb.BackupStore
	asker    tenantdb.Asker
	// Hypervisor routes. These overrides are test seams; production uses the server registry.
	hvServers hvDirectory
	openPVE   func(servers.Endpoint) (*proxmox.Client, error)
	openHV    func(servers.Endpoint) (hv.Driver, error)
	// openIncusOCIRecovery is a read-only OCI recovery seam. Production opens the
	// configured Incus endpoint; tests can assert that recovery invokes no writes.
	openIncusOCIRecovery func(servers.Endpoint) (incusOCIRecoveryProvider, error)
	// hvVisible is the tenant isolation seam. Nil uses the server registry.
	hvVisible func(ctx context.Context, tenant, id string) (servers.Endpoint, error)
}

// authTime is when the caller last signed in at the IdP (zero for PATs).
func (a *API) authTime(r *http.Request) time.Time {
	if a.authAt != nil {
		return a.authAt(r)
	}
	return a.flow.AuthTime(r)
}

func New(cfg *config.Config, flow *auth.Flow, d *db.DB) *API {
	a := &API{cfg: cfg, flow: flow, db: d, host: hostclient.New(cfg.Incus.HostdURL), started: time.Now(), probeGate: newProbeConcurrencyGate(cfg.ProbeConcurrency),
		jobs: jobs.NewStore(d.Pool), subs: &subs.Store{Pool: d.Pool}, gh: newGitHub(cfg.GitHub),
		runs: &agentruns.Store{Pool: d.Pool}, svcs: rrflowStore(d), n2: newN2(cfg, d)}
	a.tenantDB = tenantdb.NewGate(tenantdb.DialerFrom(cfg.TenantDB.Host, cfg.TenantDB.PortOrDefault(), cfg.TenantDB.SSL(), cfg.TenantDB.Timeout(), cfg.TenantDBSecrets),
		tenantdb.Guard{Timeout: cfg.TenantDB.Timeout(), MaxRows: cfg.TenantDB.Rows()})
	importCtx, cancelImport := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelImport()
	if err := a.importPlatformHealthConfig(importCtx); err != nil {
		log.Printf("platform health config import: %v", err)
	}
	return a
}

func (a *API) Register(mux Router) {
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /v1/health", a.health)
	mux.HandleFunc("GET /v1/settings/public", a.publicSettings)
	mux.HandleFunc("GET /v1/settings/ui", a.flow.Require(a.uiSettings))
	mux.HandleFunc("GET /v1/version", a.flow.Require(a.versionInfo))
	mux.HandleFunc("GET /v1/me", a.flow.Require(a.me))
	mux.HandleFunc("GET /v1/platform/services", a.flow.Require(a.services))
	mux.HandleFunc("POST /v1/tenants", a.flow.Require(a.createTenant))
	mux.HandleFunc("POST /v1/onboarding", a.flow.Require(a.onboard))
	mux.HandleFunc("GET /v1/tenants", a.flow.Require(a.listTenants))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/provision", a.flow.Require(a.retryProvision))
	mux.HandleFunc("GET /v1/tenants/{tenantId}", a.flow.Require(a.tenant))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/settings", a.flow.Require(a.tenantSettings))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/sparks", a.flow.Require(a.sparks))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/workspaces", a.flow.Require(a.listWorkspaces))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/workspaces", a.flow.Require(a.createWorkspace))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/workspaces/{workspaceId}", a.flow.Require(a.getWorkspace))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/workspaces/{workspaceId}/actions/{action}", a.flow.Require(a.workspaceAction))
	mux.HandleFunc("PUT /v1/tenants/{tenantId}/workspaces/{workspaceId}/firewall", a.flow.Require(a.putAccess("firewall")))
	mux.HandleFunc("PUT /v1/tenants/{tenantId}/workspaces/{workspaceId}/domains", a.flow.Require(a.putAccess("domains")))
	mux.HandleFunc("PUT /v1/tenants/{tenantId}/workspaces/{workspaceId}/previews", a.flow.Require(a.putPreviews))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/workspaces/{workspaceId}/listening", a.flow.Require(a.listening))
	a.panelRoutes(mux)
	mux.HandleFunc("GET /v1/tenants/{tenantId}/workspaces/{workspaceId}/discover", a.flow.Require(a.discoverProjects))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/workspaces/{workspaceId}/adopt", a.flow.Require(a.adoptProject))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/projects/{projectId}/github/publish", a.flow.Require(a.publishProject))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/access", a.flow.Require(a.tenantAccess))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/access/{grantId}/{decision}", a.flow.Require(a.decideAccess))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/audit", a.flow.Require(a.tenantAudit))
	mux.HandleFunc("GET /v1/jobs/{jobId}", a.flow.Require(a.getJob))
	mux.HandleFunc("GET /v1/templates", a.flow.Require(a.listTemplates))
	mux.HandleFunc("GET /v1/me/ssh-keys", a.flow.Require(a.listKeys))
	mux.HandleFunc("POST /v1/me/ssh-keys", a.flow.Require(a.addKey))
	mux.HandleFunc("DELETE /v1/me/ssh-keys/{keyId}", a.flow.Require(a.deleteKey))
	a.projectRoutes(mux)
	a.subscriptionRoutes(mux)
	a.connectRoutes(mux)
	a.githubRoutes(mux)
	a.promptRoutes(mux)
	a.serviceRoutes(mux)
	a.agentRoutes(mux)
	a.serverRoutes(mux)
	a.orgRoutes(mux)
	a.bridgeRoutes(mux)
	a.bundleRoutes(mux)
	a.chatRoutes(mux)
	a.setupRoutes(mux)
	a.tenantTeamRoutes(mux)
	a.databaseRoutes(mux)
	a.deploymentRoutes(mux)
	a.appsRoutes(mux)
	a.registerOperator(mux)
	a.registerDirectory(mux)
	a.infraRoutes(mux)
	mux.HandleFunc("GET /auth/ws-check", a.wsCheck)
	mux.HandleFunc("GET /auth/ws-login", a.wsLogin)
	mux.HandleFunc("GET /_shippin/auth", a.wsCode)
	mux.HandleFunc("GET /_shippin/status", a.wsStatus)
	mux.HandleFunc("GET /_shippin/popup", a.wsPopup)
	mux.HandleFunc("GET /_shippin/done", a.wsDone)
}

// SetDirectory wires the local account service.
func (a *API) SetDirectory(s *identity.Service) { a.directory = s }

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	dbOK := a.db.Pool.Ping(r.Context()) == nil
	writeJSON(w, map[string]any{"status": "ok", "db": dbOK, "uptime_s": int(time.Since(a.started).Seconds())})
}

// publicSettings is unauthenticated branding for the sign-in page of the app on r's origin.
func (a *API) publicSettings(w http.ResponseWriter, r *http.Request) {
	app, ready := "console", a.cfg.IDP.ClientID != ""
	if a.cfg.Operator.Host != "" && a.flow.IsOperatorOrigin(r) {
		app, ready = "operator", a.cfg.Operator.ClientID != ""
	}
	brand := a.cfg.Brand
	if a.directory != nil {
		if over, ok := a.directory.Branding(r.Context()); ok {
			brand = mergeBrand(brand, over)
		}
	}
	writeJSON(w, map[string]any{
		"brand": brand, "domain": a.cfg.Domain,
		"local_login": !a.cfg.Directory.UpstreamRequired, "signup": a.cfg.Directory.Signup,
		"setup_open":         a.directory != nil && !a.directory.HasOwner(r.Context()),
		"governance_domains": a.cfg.GovernanceDomains,
		"idp_host":           a.cfg.IDP.Issuer, "idp_account_url": a.idpAccountURL(r.Context()), "idp_name": a.cfg.IDP.Name, "login_ready": ready, "app": app,
		"console_url": "https://" + a.cfg.ConsoleHost, "operator_url": a.operatorURL(),
		"operator_origin_only": a.cfg.Operator.OriginOnly && a.cfg.Operator.Host != "",
		"login_providers":      loginProviders(a.cfg.IDP.Providers),
		"sign_in_providers":    signInProviders(a.cfg.Directory.Upstreams),
		"email_sign_in":        a.cfg.Mail.Enabled || os.Getenv("SHIPPIN_MAIL_SINK") != "",
	})
}

// signInProviders lists the OIDC providers people can sign in through. One with no issuer or client is
// not set up, so it is not offered.
func signInProviders(ups []config.Upstream) []map[string]string {
	out := []map[string]string{}
	for _, u := range ups {
		if u.Issuer != "" && u.ClientID != "" {
			out = append(out, map[string]string{"id": u.ID, "label": u.Label})
		}
	}
	return out
}

// loginProviders lists the upstream sign-in buttons (id and label only; IdP ids stay server-side).
func loginProviders(ps []config.IdPProvider) []map[string]string {
	out := []map[string]string{}
	for _, p := range ps {
		if p.ID != "" && p.IdPID != "" {
			label := p.Label
			if label == "" {
				label = p.ID
			}
			out = append(out, map[string]string{"id": p.ID, "label": label})
		}
	}
	return out
}
