package hostd

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func (s *Server) platformInstances(w http.ResponseWriter, r *http.Request) {
	var out json.RawMessage
	if err := query(r.Context(), fmt.Sprintf("/1.0/instances?project=%s&recursion=2", s.cfg.Platform), &out); err != nil {
		fail(w, http.StatusBadGateway, "incus unavailable")
		return
	}
	reply(w, map[string]any{"metadata": out})
}

func (s *Server) provision(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plan string `json:"plan"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	res, err := s.Provision(r.Context(), r.PathValue("t"), body.Plan)
	if err != nil {
		log.Printf("provision %s: %v", r.PathValue("t"), err)
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("provisioned tenant %s", r.PathValue("t"))
	reply(w, res)
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var body WorkspaceReq
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	defer lockWorkspace(r.PathValue("w"))()
	ip, err := s.CreateWorkspace(r.Context(), r.PathValue("t"), r.PathValue("w"), body)
	if err != nil {
		log.Printf("workspace %s: %v", r.PathValue("w"), err)
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("workspace %s ready at %s", r.PathValue("w"), ip)
	reply(w, map[string]string{"ipv4": ip})
}

func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	var body WorkspaceReq
	if r.ContentLength != 0 && json.NewDecoder(r.Body).Decode(&body) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	defer lockWorkspace(r.PathValue("w"))()
	if err := s.Action(r.Context(), r.PathValue("t"), r.PathValue("w"), r.PathValue("action"), body); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, map[string]string{"status": "ok"})
}
