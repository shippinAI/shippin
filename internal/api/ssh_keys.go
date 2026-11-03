package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostd"
)

func (a *API) userKeys(r *http.Request, sub string) ([]string, error) {
	ks, err := a.db.SSHKeys(r.Context(), sub)
	out := []string{}
	for _, k := range ks {
		out = append(out, k.PublicKey)
	}
	return out, err
}

func fingerprint(pub string) string {
	f := strings.Fields(pub)
	raw, err := base64.StdEncoding.DecodeString(f[1])
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

func (a *API) listKeys(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	ks, err := a.db.SSHKeys(r.Context(), c.id.Sub)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "SSH keys are unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"items": ks})
}

func (a *API) addKey(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	var in struct{ Name, PublicKey string }
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in)
	in.PublicKey, in.Name = strings.TrimSpace(in.PublicKey), strings.TrimSpace(in.Name)
	if _, err := hostd.ValidKeys([]string{in.PublicKey}); err != nil || in.PublicKey == "" {
		auth.Problem(w, http.StatusBadRequest, "That doesn't look like an OpenSSH public key.")
		return
	}
	if in.Name == "" || len(in.Name) > 64 {
		auth.Problem(w, http.StatusBadRequest, "Give the key a name (up to 64 characters).")
		return
	}
	id, _ := uuid.NewV7()
	k := db.SSHKey{ID: id.String(), Name: in.Name, PublicKey: in.PublicKey, Fingerprint: fingerprint(in.PublicKey)}
	if err := a.db.AddSSHKey(r.Context(), c.id.Sub, k); err != nil {
		auth.Problem(w, http.StatusConflict, "This key is already on your account.")
		return
	}
	go a.pushKeys(c.id.Sub)
	writeStatus(w, http.StatusCreated, k)
}

func (a *API) deleteKey(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	if found, err := a.db.DeleteSSHKey(r.Context(), c.id.Sub, r.PathValue("keyId")); err != nil || !found {
		auth.Problem(w, http.StatusNotFound, "Key not found.")
		return
	}
	go a.pushKeys(c.id.Sub)
	w.WriteHeader(http.StatusNoContent)
}

// pushKeys applies the user's current keys to every running workspace they created.
func (a *API) pushKeys(sub string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ks, err := a.db.SSHKeys(ctx, sub)
	if err != nil {
		return
	}
	keys := []string{}
	for _, k := range ks {
		keys = append(keys, k.PublicKey)
	}
	wss, _ := a.db.WorkspacesByCreator(ctx, sub)
	for _, ws := range wss {
		_ = a.db.SetWorkspaceKeys(ctx, ws.ID, keys)
		if ws.State == "running" {
			ws.SSHKeys = keys
			t := tenantView{Tenant: db.Tenant{ID: ws.TenantID}}
			_ = a.host.Action(ctx, ws.TenantID, ws.ID, "keys", a.hostReq(&t, ws))
		}
	}
}
