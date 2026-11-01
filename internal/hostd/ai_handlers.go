package hostd

import (
	"encoding/json"
	"log"
	"net/http"
)

// aiRoutes: per-user AI homes and activation containers (M2; {a} is the console account id), and
// the M3 git token refresh.
func (s *Server) aiRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /tenants/{t}/workspaces/{w}/ai-home", s.withTenant(s.attachAIHome))
	m.HandleFunc("GET /tenants/{t}/workspaces/{w}/ai-status", s.withTenant(s.workspaceAIStatus))
	m.HandleFunc("PUT /tenants/{t}/workspaces/{w}/git-token", s.withTenant(s.putGitToken))
	m.HandleFunc("POST /tenants/{t}/utilities/{a}/start", s.withAccount(s.startUtility))
	m.HandleFunc("POST /tenants/{t}/utilities/{a}/stop", s.withAccount(s.stopUtility))
	m.HandleFunc("POST /tenants/{t}/utilities/{a}/logout", s.withAccount(s.logoutUtility))
	m.HandleFunc("GET /tenants/{t}/utilities/{a}/ai-status", s.withAccount(s.utilityAIStatus))
}

func (s *Server) withAccount(h http.HandlerFunc) http.HandlerFunc {
	return s.withTenant(func(w http.ResponseWriter, r *http.Request) {
		if ValidUUID(r.PathValue("a")) != nil {
			fail(w, http.StatusBadRequest, "invalid account id")
			return
		}
		defer lockWorkspace("u-" + r.PathValue("a"))()
		h(w, r)
	})
}

func (s *Server) attachAIHome(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Account string `json:"account"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || ValidUUID(body.Account) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	defer lockWorkspace(r.PathValue("w"))()
	if err := s.AttachAIHome(r.Context(), r.PathValue("t"), r.PathValue("w"), body.Account); err != nil {
		log.Printf("ai home %s: %v", r.PathValue("w"), err)
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, map[string]string{"status": "ok"})
}

func (s *Server) workspaceAIStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.WorkspaceAIStatus(r.Context(), r.PathValue("t"), r.PathValue("w"))
	replyStatus(w, st, err)
}

func (s *Server) utilityAIStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.UtilityAIStatus(r.Context(), r.PathValue("t"), r.PathValue("a"))
	replyStatus(w, st, err)
}

func replyStatus(w http.ResponseWriter, st *AIStatus, err error) {
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, st)
}

func (s *Server) startUtility(w http.ResponseWriter, r *http.Request) {
	var body UtilityReq
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	ip, err := s.StartUtility(r.Context(), r.PathValue("t"), r.PathValue("a"), body)
	if err != nil {
		log.Printf("utility %s: %v", r.PathValue("a"), err)
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	reply(w, map[string]string{"ipv4": ip})
}

func (s *Server) stopUtility(w http.ResponseWriter, r *http.Request) {
	if err := s.StopUtility(r.Context(), r.PathValue("t"), r.PathValue("a")); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, map[string]string{"status": "ok"})
}

func (s *Server) logoutUtility(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	if err := s.UtilityLogout(r.Context(), r.PathValue("t"), r.PathValue("a"), body.Provider); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, map[string]string{"status": "ok"})
}
