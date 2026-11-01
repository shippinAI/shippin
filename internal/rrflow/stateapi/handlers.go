package stateapi

import (
	"net/http"

	"shippin.cloud/console/internal/rrflow/state"
)

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, ws string) {
	var in struct{ Agent, Mode string }
	if !decode(w, r, &in) {
		return
	}
	x, err := store(r).StartSession(ws, in.Agent, in.Mode)
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusCreated, x)
}

func (s *Server) endSession(w http.ResponseWriter, r *http.Request, ws string) {
	var in struct{ Status string }
	if !decode(w, r, &in) {
		return
	}
	x, err := store(r).EndSession(ws, r.PathValue("id"), in.Status)
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusOK, x)
}

func (s *Server) getMemory(w http.ResponseWriter, r *http.Request, ws string) {
	x, err := store(r).GetMemory(scope(ws), r.PathValue("key"))
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusOK, x)
}

func (s *Server) putMemory(w http.ResponseWriter, r *http.Request, ws string) {
	var in struct{ Value string }
	if !decode(w, r, &in) {
		return
	}
	x, err := store(r).PutMemory(scope(ws), r.PathValue("key"), in.Value)
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusOK, x)
}

func (s *Server) deleteMemory(w http.ResponseWriter, r *http.Request, ws string) {
	if err := store(r).DeleteMemory(scope(ws), r.PathValue("key")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// addDecision appends a decision row for the caller's workspace. The actor defaults to the
// workspace's agent; workspace and org always come from the binding, never from the body.
func (s *Server) addDecision(w http.ResponseWriter, r *http.Request, ws string) {
	var in struct {
		Session, Actor, Action, Resource, Effect, Reason string
		ConfigVersion                                    string `json:"config_version"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Actor == "" {
		in.Actor = "agent:" + ws
	}
	x, err := store(r).AppendDecision(state.Decision{Workspace: ws, Session: in.Session, Actor: in.Actor, Action: in.Action,
		Resource: in.Resource, Effect: in.Effect, Reason: in.Reason, ConfigVer: in.ConfigVersion})
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusCreated, x)
}
