package hostd

import (
	"encoding/json"
	"net/http"
)

type Server struct{ cfg *Config }

func NewServer(cfg *Config) *Server { return &Server{cfg: cfg} }

// Routes: the full, closed set of operations this helper performs.
func (s *Server) Routes() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("GET /platform/instances", s.platformInstances)
	m.HandleFunc("GET /plans/{plan}", s.plan)
	m.HandleFunc("GET /host/stats", s.hostStats)
	m.HandleFunc("GET /edge/routes", s.edgeRoutes)
	m.HandleFunc("GET /edge/ports", s.edgePorts)
	m.HandleFunc("GET /networks/tenants", s.tenantNetworks)
	m.HandleFunc("POST /tenants/{t}/provision", s.withTenant(s.provision))
	m.HandleFunc("DELETE /tenants/{t}", s.withTenant(s.deleteTenant))
	m.HandleFunc("GET /tenants/{t}/instances", s.withTenant(s.tenantInstances))
	m.HandleFunc("GET /tenants/{t}/vm", s.withTenant(s.tenantVM))
	m.HandleFunc("GET /tenants/{t}/metrics", s.withTenant(s.tenantMetrics))
	m.HandleFunc("GET /tenants/{t}/storage", s.withTenant(s.tenantStorage))
	m.HandleFunc("POST /tenants/{t}/storage", s.withTenant(s.growStorage))
	m.HandleFunc("POST /tenants/{t}/plan", s.withTenant(s.applyPlan))
	m.HandleFunc("POST /tenants/{t}/vm", s.withTenant(s.enableVM))
	m.HandleFunc("POST /tenants/{t}/vm/{action}", s.withTenant(s.vmAction))
	m.HandleFunc("POST /tenants/{t}/desktop/prepare", s.withTenant(s.desktopPrepare))
	m.HandleFunc("GET /tenants/{t}/apps/status", s.withTenant(s.appsStatus))
	m.HandleFunc("GET /tenants/{t}/desktop", s.withTenant(s.desktopTunnel))
	m.HandleFunc("POST /tenants/{t}/inner-projects/harden", s.withTenant(s.hardenInner))
	m.HandleFunc("POST /tenants/{t}/workspaces/{w}", s.withTenant(s.createWorkspace))
	m.HandleFunc("POST /tenants/{t}/workspaces/{w}/{action}", s.withTenant(s.action))
	m.HandleFunc("GET /tenants/{t}/workspaces/{w}/listening", s.withTenant(s.listening))
	m.HandleFunc("GET /tenants/{t}/workspaces/{w}/discover", s.withTenant(s.discover))
	m.HandleFunc("POST /tenants/{t}/workspaces/{w}/git", s.withTenant(s.gitOp))
	m.HandleFunc("GET /tenants/{t}/workspaces/{w}/relay", s.withTenant(s.relay))
	m.HandleFunc("POST /tenants/{t}/starter-db", s.withTenant(s.starterDB))
	m.HandleFunc("POST /tenants/{t}/workspaces/{w}/chat/{cmd}", s.withTenant(s.chatExec))
	m.HandleFunc("GET /tenants/{t}/workspaces/{w}/progress", s.withTenant(func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]string{"progress": Progress(r.PathValue("w"))})
	}))
	s.aiRoutes(m)
	s.connectRoutes(m)
	s.agentRoutes(m)
	s.serviceRoutes(m)
	s.siteRoutes(m)
	return m
}

func (s *Server) withTenant(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ValidUUID(r.PathValue("t")) != nil {
			fail(w, http.StatusBadRequest, "invalid tenant id")
			return
		}
		if id := r.PathValue("w"); id != "" && ValidUUID(id) != nil {
			fail(w, http.StatusBadRequest, "invalid workspace id")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		h(w, r)
	}
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *Server) plan(w http.ResponseWriter, r *http.Request) {
	p, ok := s.cfg.Plans[r.PathValue("plan")]
	if !ok {
		fail(w, http.StatusNotFound, "unknown plan")
		return
	}
	reply(w, p)
}
