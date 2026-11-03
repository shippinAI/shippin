package hostd

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// tenantInstances lists host-project instances (legacy direct) plus, for a VM-backed tenant, the
// containers nested in its VM. The tenant VM itself is reported by GET /tenants/{t}/vm.
func (s *Server) tenantInstances(w http.ResponseWriter, r *http.Request) {
	t := r.PathValue("t")
	var host []map[string]any
	if err := query(r.Context(), fmt.Sprintf("/1.0/instances?project=%s&recursion=2", Project(t)), &host); err != nil || host == nil {
		fail(w, http.StatusNotFound, "tenant project not found")
		return
	}
	out := make([]map[string]any, 0, len(host))
	for _, i := range host {
		if i["name"] != TenantVM(t) {
			out = append(out, i)
		}
	}
	// Inside a tenant VM, workspaces live in per-project pj-* projects: list every project.
	// partial tells callers (drift checks) the inner list couldn't be read, so absence means nothing.
	partial := false
	if at, err := s.newPlace(r.Context(), t); err == nil && at.vm {
		var inner []map[string]any
		// queryAll, not query: Incus 6.0 returns 400 when all-projects is combined with project=,
		// and place.query always appends project=<this place>.
		// A nil list is not a confirmed empty inventory.
		if at.queryAll(r.Context(), "/1.0/instances?recursion=2", &inner) == nil && inner != nil {
			out = append(out, inner...)
		} else {
			partial = true
		}
	} else if err != nil {
		partial = true
	}
	reply(w, map[string]any{"metadata": out, "partial": partial})
}

func (s *Server) tenantStorage(w http.ResponseWriter, r *http.Request) {
	rep, err := s.StorageReport(r.Context(), r.PathValue("t"))
	if err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	reply(w, rep)
}

func (s *Server) growStorage(w http.ResponseWriter, r *http.Request) {
	var tgt StorageTarget
	if json.NewDecoder(r.Body).Decode(&tgt) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	rep, err := s.GrowStorage(r.Context(), r.PathValue("t"), tgt)
	if err != nil {
		code := http.StatusConflict
		msg := err.Error()
		if strings.Contains(msg, "set quota") || strings.Contains(msg, "sizes must") || strings.Contains(msg, "belongs") ||
			strings.Contains(msg, "shrinking") || strings.Contains(msg, "must cover") || strings.Contains(msg, "positive") {
			code = http.StatusBadRequest
		}
		fail(w, code, msg)
		return
	}
	log.Printf("storage grow %s", r.PathValue("t"))
	reply(w, rep)
}

func (s *Server) applyPlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plan string `json:"plan"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Plan == "" {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	rep, err := s.ApplyPlan(r.Context(), r.PathValue("t"), body.Plan)
	if err != nil {
		code := http.StatusConflict
		msg := err.Error()
		if strings.Contains(msg, "unknown plan") || strings.Contains(msg, "refusing") || strings.Contains(msg, "shrinking") {
			code = http.StatusBadRequest
		}
		fail(w, code, msg)
		return
	}
	log.Printf("plan %s applied to %s", body.Plan, r.PathValue("t"))
	reply(w, rep)
}

func (s *Server) tenantVM(w http.ResponseWriter, r *http.Request) {
	v, err := s.VMView(r.Context(), r.PathValue("t"))
	if err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	reply(w, v)
}

// enableVM turns an existing (legacy) tenant VM-backed. Existing workspaces stay legacy direct.
func (s *Server) enableVM(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plan string `json:"plan"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	t := r.PathValue("t")
	if p, ok := s.cfg.Plans[body.Plan]; !ok || p.VM == nil {
		fail(w, http.StatusBadRequest, "plan has no tenant VM size")
		return
	}
	if _, err := incus(r.Context(), "project", "set", Project(t), modeKey+"="+modeVM); err != nil {
		fail(w, http.StatusNotFound, "tenant project not found")
		return
	}
	if _, err := s.Provision(r.Context(), t, body.Plan); err != nil {
		log.Printf("enable tenant VM %s: %v", t, err)
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("tenant VM enabled for %s", t)
	s.tenantVM(w, r)
}

func (s *Server) vmAction(w http.ResponseWriter, r *http.Request) {
	t, pr, vm := r.PathValue("t"), "--project="+Project(r.PathValue("t")), TenantVM(r.PathValue("t"))
	var err error
	switch r.PathValue("action") {
	case "start":
		if _, err = incus(r.Context(), "start", vm, pr); isRunning(err) {
			err = nil
		}
		if err == nil {
			err = waitAgent(r.Context(), t)
		}
		if err == nil {
			err = waitInner(r.Context(), t)
		}
	case "stop":
		_, err = incus(r.Context(), "stop", vm, pr, "--timeout=60")
	case "restart":
		if _, err = incus(r.Context(), "restart", vm, pr, "--timeout=60"); err == nil {
			err = waitAgent(r.Context(), t)
		}
		if err == nil {
			err = waitInner(r.Context(), t)
		}
	default:
		fail(w, http.StatusBadRequest, "unknown action")
		return
	}
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	log.Printf("tenant VM %s: %s", t, r.PathValue("action"))
	reply(w, map[string]string{"status": "ok"})
}
