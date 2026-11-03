package hostd

import (
	"encoding/json"
	"log"
	"net/http"
)

// hardenInner is POST /tenants/{t}/inner-projects/harden {plan, restart}.
func (s *Server) hardenInner(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plan    string `json:"plan"`
		Restart bool   `json:"restart"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	plan, ok := s.cfg.Plans[body.Plan]
	if !ok {
		fail(w, http.StatusBadRequest, "unknown plan")
		return
	}
	t := r.PathValue("t")
	res, err := s.HardenInner(r.Context(), t, plan, body.Restart)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	log.Printf("tenant %s: inner projects hardened (%d checked, restart=%v)", t, len(res), body.Restart)
	reply(w, map[string]any{"projects": res})
}
