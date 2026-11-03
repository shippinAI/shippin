package hostd

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// serviceRoutes: managed service instances (rrflow/state first) under a project on the boundary VM.
func (s *Server) serviceRoutes(m *http.ServeMux) {
	base := "/tenants/{t}/projects/{pj}/services/{si}"
	m.HandleFunc("POST "+base+"/rrflow", s.withService(s.ensureRRFlow))
	m.HandleFunc("DELETE "+base, s.withService(s.deleteService))
	m.HandleFunc("POST "+base+"/bindings/{sb}", s.withService(s.bindService))
	m.HandleFunc("DELETE "+base+"/bindings/{sb}", s.withService(s.unbindService))
	m.HandleFunc("GET "+base+"/state/{kind}", s.withService(s.serviceState))
	m.HandleFunc("POST "+base+"/migrate-to/{to}", s.withService(s.migrateService))
	m.HandleFunc("POST "+base+"/retire-into/{to}", s.withService(s.migrateService))
}

func (s *Server) withService(h http.HandlerFunc) http.HandlerFunc {
	return s.withTenant(func(w http.ResponseWriter, r *http.Request) {
		for _, k := range []string{"pj", "si"} {
			if ValidUUID(r.PathValue(k)) != nil {
				fail(w, http.StatusBadRequest, "invalid "+k)
				return
			}
		}
		if sb := r.PathValue("sb"); sb != "" && ValidUUID(sb) != nil {
			fail(w, http.StatusBadRequest, "invalid binding id")
			return
		}
		defer lockWorkspace("si-" + r.PathValue("si"))()
		h(w, r)
	})
}

func (s *Server) ensureRRFlow(w http.ResponseWriter, r *http.Request) {
	var body ServiceReq
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	body.Project, body.Shared = r.PathValue("pj"), body.Shared || shared(r)
	v, err := s.EnsureRRFlow(r.Context(), r.PathValue("t"), r.PathValue("si"), body)
	if err != nil {
		log.Printf("service %s: %v", r.PathValue("si"), err)
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, v)
}

// deleteService is refused unless services.rrflow_state.allow_delete; every call is journaled as
// "hostd: service DELETE ..." so a change window can prove zero deletes (GC-20261019-PB B5).
func (s *Server) deleteService(w http.ResponseWriter, r *http.Request) {
	si := r.PathValue("si")
	if c := s.cfg.Services.RRFlow; c == nil || !c.AllowDelete {
		log.Printf("hostd: service DELETE si-%s refused (allow_delete off)", si)
		fail(w, http.StatusForbidden, "service delete is disabled (services.rrflow_state.allow_delete)")
		return
	}
	log.Printf("hostd: service DELETE si-%s", si)
	if err := s.DeleteService(r.Context(), r.PathValue("t"), r.PathValue("pj"), r.PathValue("si"), shared(r)); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, map[string]string{"status": "ok"})
}

func (s *Server) bindService(w http.ResponseWriter, r *http.Request) {
	var body BindReq
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	body.Project, body.Shared = r.PathValue("pj"), body.Shared || shared(r)
	if ValidUUID(body.Workspace) == nil {
		defer lockWorkspace(body.Workspace)()
	}
	if err := s.BindRRFlow(r.Context(), r.PathValue("t"), r.PathValue("si"), r.PathValue("sb"), body); err != nil {
		log.Printf("binding %s: %v", r.PathValue("sb"), err)
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, map[string]string{"status": "ok"})
}

func (s *Server) unbindService(w http.ResponseWriter, r *http.Request) {
	body := BindReq{Project: r.PathValue("pj"), Workspace: r.URL.Query().Get("workspace_id"), Shared: shared(r)}
	if err := s.UnbindRRFlow(r.Context(), r.PathValue("t"), r.PathValue("si"), r.PathValue("sb"), body); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, map[string]string{"status": "ok"})
}

func (s *Server) serviceState(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	out, err := s.RRFlowExport(r.Context(), r.PathValue("t"), r.PathValue("pj"), r.PathValue("si"), r.PathValue("kind"), n, shared(r))
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

// shared: the request addresses the tenant VM's one rrflow instance (Notion D36), ?scope=vm.
func shared(r *http.Request) bool { return r.URL.Query().Get("scope") == "vm" }

// migrateService: migrate-to copies a pre-D36 instance's state into namespace pj of the VM-wide
// instance {to} (snapshot first, no delete); retire-into deletes the source once {to} verifies.
func (s *Server) migrateService(w http.ResponseWriter, r *http.Request) {
	to := r.PathValue("to")
	if ValidUUID(to) != nil {
		fail(w, http.StatusBadRequest, "invalid target")
		return
	}
	defer lockWorkspace("si-" + to)()
	run := s.MigrateRRFlow
	if strings.Contains(r.URL.Path, "/retire-into/") {
		run = s.RetireRRFlow // a separate call, never in the same run as the copy
		log.Printf("hostd: service retire-into si-%s -> si-%s requested", r.PathValue("si"), to)
	}
	if err := run(r.Context(), r.PathValue("t"), r.PathValue("pj"), r.PathValue("si"), to); err != nil {
		log.Printf("service %s migrate: %v", r.PathValue("si"), err)
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, map[string]string{"status": "ok"})
}
