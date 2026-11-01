package hostd

import (
	"encoding/json"
	"log"
	"net/http"
)

// agentRoutes (M4): the prompt file, the agent start and its status. No route takes a command.
func (s *Server) agentRoutes(m *http.ServeMux) {
	m.HandleFunc("PUT /tenants/{t}/workspaces/{w}/prompt", s.withTenant(s.putPrompt))
	m.HandleFunc("POST /tenants/{t}/workspaces/{w}/agent", s.withTenant(s.startAgent))
	m.HandleFunc("GET /tenants/{t}/workspaces/{w}/agent", s.withTenant(s.agentStatus))
}

func (s *Server) putPrompt(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prompt string `json:"prompt"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	defer lockWorkspace(r.PathValue("w"))()
	if err := s.PutPrompt(r.Context(), r.PathValue("t"), r.PathValue("w"), body.Prompt); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, map[string]string{"status": "ok"})
}

func (s *Server) startAgent(w http.ResponseWriter, r *http.Request) {
	var body AgentReq
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	defer lockWorkspace(r.PathValue("w"))()
	started, err := s.StartAgent(r.Context(), r.PathValue("t"), r.PathValue("w"), body)
	if err != nil {
		log.Printf("agent %s: %v", r.PathValue("w"), err)
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, map[string]bool{"started": started})
}

func (s *Server) agentStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.ReadAgentStatus(r.Context(), r.PathValue("t"), r.PathValue("w"))
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, st)
}
