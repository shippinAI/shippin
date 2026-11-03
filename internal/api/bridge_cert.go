package api

import (
	"encoding/json"
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/sshca"
)

// bridgeCert signs the bridge's ephemeral public key for one workspace: principal
// <prefix><workspace uuid>, key id = account uuid + email + session, TTL bridge.cert_ttl_seconds.
// The certificate is audited before it is issued.
func (a *API) bridgeCert(w http.ResponseWriter, r *http.Request) {
	c, ses, ok := a.bridgeCaller(w, r)
	if !ok {
		return
	}
	ws, t, ok := a.bridgeWorkspace(w, r, c, r.PathValue("id"))
	if !ok {
		return
	}
	var in struct {
		PublicKey string `json:"public_key"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in) != nil || in.PublicKey == "" {
		auth.Problem(w, http.StatusBadRequest, "public_key is required.")
		return
	}
	b := a.n2.br
	principal := b.cfg.PrincipalPrefix + ws.ID
	keyID := ses.Sub + " " + ses.Email + " session=" + ses.ID
	e := db.AuditEntry{TenantID: t.ID, TenantName: t.Name, WorkspaceID: ws.ID, WorkspaceName: ws.Name, Actor: c.id.Sub,
		ActorName: c.id.Name, Action: "bridge.cert_issued", Detail: "principal " + principal + " ttl " + b.certTTL().String() + " session " + ses.ID, IP: clientIP(r)}
	if !a.auditFirst(w, r, e) {
		return
	}
	cert, until, err := b.ca.Sign(sshca.Request{PublicKey: in.PublicKey, Principal: principal, KeyID: keyID, TTL: b.certTTL()})
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"certificate": cert, "valid_before": until, "principal": principal,
		"ssh_user": b.cfg.SSHUser, "relay_path": "/v1/bridge/workspaces/" + ws.ID + "/relay"})
}
