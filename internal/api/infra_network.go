package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/networking"
)

type networkConnectionBody struct {
	Label               string          `json:"label"`
	Provider            string          `json:"provider"`
	Config              json.RawMessage `json:"config"`
	Credential          string          `json:"credential,omitempty"`
	ClearCredential     bool            `json:"clear_credential,omitempty"`
	SyncIntervalSeconds int             `json:"sync_interval_seconds,omitempty"`
}

func (b networkConnectionBody) connection(id string) networking.Connection {
	interval := b.SyncIntervalSeconds
	if interval == 0 {
		interval = 300
	}
	return networking.Connection{ID: id, Label: strings.TrimSpace(b.Label), Provider: b.Provider, Config: b.Config, SyncIntervalSeconds: interval}
}

func (a *API) networkStore() *networking.Store {
	if a.db == nil || a.db.Pool == nil {
		return nil
	}
	return &networking.Store{Pool: a.db.Pool}
}

func (a *API) requireNetworkStore(w http.ResponseWriter) *networking.Store {
	s := a.networkStore()
	if s == nil {
		authUnavailable(w, "Network integrations are unavailable until the platform database is connected.")
		return nil
	}
	return s
}

func (a *API) infraNetworkConnections(w http.ResponseWriter, r *http.Request, _ *caller) {
	s := a.requireNetworkStore(w)
	if s == nil {
		return
	}
	items, err := s.List(r.Context())
	if err != nil {
		authUnavailable(w, "Network integrations are unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) infraNetworkTest(w http.ResponseWriter, r *http.Request, _ *caller) {
	var b networkConnectionBody
	if !decodeStrict(w, r, &b) {
		return
	}
	c := b.connection(uuid.NewString())
	if err := c.Validate(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if needsCredential(c.Provider) && strings.TrimSpace(b.Credential) == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "A provider credential is required to test this connection."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	items, err := networking.Inventory(ctx, c, []byte(b.Credential))
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "resource_count": len(items), "resources": items})
}

func (a *API) infraNetworkCreate(w http.ResponseWriter, r *http.Request, c *caller) {
	var b networkConnectionBody
	if !decodeStrict(w, r, &b) {
		return
	}
	id := uuid.NewString()
	item := b.connection(id)
	if err := item.Validate(); err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	if needsCredential(item.Provider) && strings.TrimSpace(b.Credential) == "" {
		auth.Problem(w, http.StatusBadRequest, "A credential is required for this provider.")
		return
	}
	var sealed []byte
	if b.Credential != "" {
		s, err := a.sealer()
		if err != nil {
			auth.Problem(w, http.StatusServiceUnavailable, "The installation cannot seal provider credentials.")
			return
		}
		sealed, err = s.Seal([]byte(b.Credential), id)
		if err != nil {
			auth.Problem(w, http.StatusInternalServerError, "The provider credential could not be protected.")
			return
		}
	}
	s := a.requireNetworkStore(w)
	if s == nil {
		return
	}
	saved, err := s.Create(r.Context(), item, sealed, c.id.Sub)
	if err != nil {
		log.Printf("network integration create: %v", err)
		auth.Problem(w, http.StatusConflict, "The network integration could not be saved.")
		return
	}
	item = saved
	writeStatus(w, http.StatusCreated, item)
}

func (a *API) infraNetworkUpdate(w http.ResponseWriter, r *http.Request, _ *caller) {
	var b networkConnectionBody
	if !decodeStrict(w, r, &b) {
		return
	}
	id := r.PathValue("id")
	item := b.connection(id)
	if err := item.Validate(); err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	store := a.requireNetworkStore(w)
	if store == nil {
		return
	}
	old, err := store.Get(r.Context(), id)
	if err != nil {
		infraErr(w, err)
		return
	}
	if item.Provider != old.Provider {
		auth.Problem(w, http.StatusBadRequest, "Provider type cannot be changed; remove and add a new integration.")
		return
	}
	var sealed []byte
	if b.Credential != "" {
		s, e := a.sealer()
		if e != nil {
			auth.Problem(w, http.StatusServiceUnavailable, "The installation cannot seal provider credentials.")
			return
		}
		sealed, e = s.Seal([]byte(b.Credential), id)
		if e != nil {
			auth.Problem(w, http.StatusInternalServerError, "The provider credential could not be protected.")
			return
		}
	} else if needsCredential(item.Provider) && (!old.HasCredential || b.ClearCredential) {
		auth.Problem(w, http.StatusBadRequest, "A credential is required for this provider.")
		return
	}
	item, err = store.Update(r.Context(), item, sealed, b.ClearCredential)
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, item)
}

func (a *API) infraNetworkDelete(w http.ResponseWriter, r *http.Request, _ *caller) {
	s := a.requireNetworkStore(w)
	if s == nil {
		return
	}
	if err := s.Delete(r.Context(), r.PathValue("id")); err != nil {
		infraErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) infraNetworkResources(w http.ResponseWriter, r *http.Request, _ *caller) {
	s := a.requireNetworkStore(w)
	if s == nil {
		return
	}
	items, err := s.Resources(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) infraNetworkSync(w http.ResponseWriter, r *http.Request, _ *caller) {
	store := a.requireNetworkStore(w)
	if store == nil {
		return
	}
	item, err := store.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		infraErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	err = a.syncNetworkConnection(ctx, store, item)
	if err != nil {
		infraErr(w, err)
		return
	}
	resources, err := store.Resources(ctx, item.ID)
	if err != nil {
		authUnavailable(w, "The observed inventory could not be read.")
		return
	}
	writeJSON(w, map[string]any{"items": resources, "checked_at": time.Now().UTC()})
}

func needsCredential(provider string) bool {
	return provider == networking.CloudflareTunnel || provider == networking.Tailscale
}

func safeProviderError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 300 {
		message = message[:300]
	}
	if message == "" {
		return "provider request failed"
	}
	return message
}

func authUnavailable(w http.ResponseWriter, msg string) {
	auth.Problem(w, http.StatusServiceUnavailable, msg)
}
