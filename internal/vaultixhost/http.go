package vaultixhost

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"

	"shippin.cloud/console/internal/vaultix"
)

// Who resolves the signed-in person (the console's Nomen session) or writes the error.
type Who func(w http.ResponseWriter, r *http.Request) (Person, bool)

// Mux is what routes register on (the console wraps it so every route needs a session).
type Mux interface {
	HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request))
}

// Handler serves /v1/vaultix/*. Every call acts only inside the caller's own organization.
type Handler struct {
	Svc *Service
	Who Who
}

var keyRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func (h *Handler) Register(m Mux) {
	m.HandleFunc("GET /v1/vaultix/account", h.with(h.account))
	m.HandleFunc("GET /v1/vaultix/secrets", h.with(h.list))
	m.HandleFunc("PUT /v1/vaultix/secrets/{key}", h.with(h.put))
	m.HandleFunc("DELETE /v1/vaultix/secrets/{key}", h.with(h.del))
	m.HandleFunc("POST /v1/vaultix/secrets/{key}/reveal", h.with(h.reveal))
	m.HandleFunc("POST /v1/vaultix/keyring", h.with(h.keyring))
	m.HandleFunc("POST /v1/vaultix/import", h.with(h.importCloud))
}

type call struct {
	w http.ResponseWriter
	r *http.Request
	a *Account
	c *vaultix.Client
}

func (h *Handler) with(fn func(call)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := h.Who(w, r)
		if !ok {
			return
		}
		a, err := h.Svc.Ensure(r.Context(), p)
		if err != nil {
			log.Printf("vaultix: provision %s: %v", p.Sub, err)
			problem(w, http.StatusBadGateway, "Vaultix is not reachable right now. Try again shortly.")
			return
		}
		c, err := h.Svc.Client(a)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Your Vaultix credentials could not be opened.")
			return
		}
		fn(call{w, r, a, c})
	}
}

func (h *Handler) scope(x call) (vaultix.Scope, bool) {
	env := x.r.URL.Query().Get("env")
	for _, e := range h.Svc.Envs {
		if e == env {
			return vaultix.Scope{Project: x.a.Project, Env: env, Path: "/"}, true
		}
	}
	problem(x.w, http.StatusBadRequest, "Unknown environment.")
	return vaultix.Scope{}, false
}

func (h *Handler) account(x call) {
	reply(x.w, map[string]any{"org": x.a.Org, "workspace": x.a.Project, "workspace_slug": "ws-" + x.a.Sub, "email": x.a.Email, "envs": h.Svc.Envs,
		"keyring_connected": x.a.KeyringID != "", "api_url": h.Svc.PublicURL})
}

func (h *Handler) list(x call) {
	s, ok := h.scope(x)
	if !ok {
		return
	}
	m, err := x.c.List(x.r.Context(), s)
	if err != nil {
		upstream(x.w, err)
		return
	}
	if m == nil {
		m = []vaultix.Meta{}
	}
	reply(x.w, map[string]any{"secrets": m})
}

func (h *Handler) put(x call) {
	s, ok := h.scope(x)
	key := x.r.PathValue("key")
	var in struct {
		Value string `json:"value"`
	}
	if !ok {
		return
	}
	if !keyRE.MatchString(key) || json.NewDecoder(http.MaxBytesReader(x.w, x.r.Body, 64<<10)).Decode(&in) != nil {
		problem(x.w, http.StatusBadRequest, "Names use letters, digits and _; values up to 64 KB.")
		return
	}
	m, err := x.c.Set(x.r.Context(), s, key, in.Value, false)
	if errors.Is(err, vaultix.ErrNotFound) || (err != nil && isMissing(err)) {
		m, err = x.c.Set(x.r.Context(), s, key, in.Value, true)
	}
	if err != nil {
		upstream(x.w, err)
		return
	}
	reply(x.w, m)
}

func (h *Handler) del(x call) {
	if s, ok := h.scope(x); ok {
		if err := x.c.Delete(x.r.Context(), s, x.r.PathValue("key")); err != nil {
			upstream(x.w, err)
			return
		}
		x.w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) reveal(x call) {
	if s, ok := h.scope(x); ok {
		_, v, err := x.c.Reveal(x.r.Context(), s, x.r.PathValue("key"))
		if err != nil {
			upstream(x.w, err)
			return
		}
		x.w.Header().Set("Cache-Control", "no-store")
		reply(x.w, map[string]string{"value": v})
	}
}
