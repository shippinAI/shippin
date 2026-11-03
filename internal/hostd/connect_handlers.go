package hostd

import (
	"encoding/json"
	"net/http"
)

// connectRoutes: "Connect accounts" for one workspace. Tokens only ever arrive in a JSON body
// and leave hostd on a container's stdin; responses carry status, URLs and one-time codes only.
func (s *Server) connectRoutes(m *http.ServeMux) {
	base := "/tenants/{t}/workspaces/{w}/connect"
	m.HandleFunc("GET "+base, s.withTenant(s.connectStatus))
	m.HandleFunc("POST "+base+"/{p}/login", s.withTenant(s.connectLogin))
	m.HandleFunc("GET "+base+"/{p}/login", s.withTenant(s.connectLoginState))
	m.HandleFunc("POST "+base+"/{p}/code", s.withTenant(s.connectCode))
	m.HandleFunc("PUT "+base+"/{p}/token", s.withTenant(s.connectToken))
	m.HandleFunc("DELETE "+base+"/{p}", s.withTenant(s.connectDisconnect))
	m.HandleFunc("GET /tenants/{t}/workspaces/{w}/terminal", s.withTenant(s.terminal))
}

func (s *Server) connectStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.ConnectStatusOf(r.Context(), r.PathValue("t"), r.PathValue("w"))
	answer(w, st, err)
}

func (s *Server) connectLogin(w http.ResponseWriter, r *http.Request) {
	st, err := s.StartLogin(r.Context(), r.PathValue("t"), r.PathValue("w"), r.PathValue("p"))
	answer(w, st, err)
}

func (s *Server) connectLoginState(w http.ResponseWriter, r *http.Request) {
	st, err := s.Login(r.PathValue("w"), r.PathValue("p"))
	answer(w, st, err)
}

func (s *Server) connectCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	answer(w, map[string]string{"status": "ok"}, s.SubmitCode(r.PathValue("w"), r.PathValue("p"), in.Code))
}

func (s *Server) connectToken(w http.ResponseWriter, r *http.Request) {
	var in SpoolReq
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		fail(w, http.StatusBadRequest, "bad body")
		return
	}
	err := s.PutToken(r.Context(), r.PathValue("t"), r.PathValue("w"), r.PathValue("p"), in)
	answer(w, map[string]string{"status": "ok"}, err)
}

func (s *Server) connectDisconnect(w http.ResponseWriter, r *http.Request) {
	err := s.Disconnect(r.Context(), r.PathValue("t"), r.PathValue("w"), r.PathValue("p"))
	answer(w, map[string]string{"status": "ok"}, err)
}

func answer(w http.ResponseWriter, v any, err error) {
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	reply(w, v)
}
