package api

import (
	"encoding/json"
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
)

type connectionView struct {
	config.ConnectProvider
	Status    string     `json:"status"` // connected | expiring | not_connected
	Method    string     `json:"method,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Stored    bool       `json:"stored_in_vaultix"`
}

type connectionsOut struct {
	CheckedAt   time.Time        `json:"checked_at"`
	Running     bool             `json:"running"`
	IDEURL      string           `json:"ide_terminal_url"`
	Connections []connectionView `json:"connections"`
}

// listConnections merges the workspace writer's report with what Vaultix holds (names only).
func (a *API) listConnections(w http.ResponseWriter, r *http.Request) {
	x, ok := a.connectFor(w, r, false)
	if !ok {
		return
	}
	out := connectionsOut{Running: x.ws.State == "running", IDEURL: a.ideTerminalURL(x.ws.URL), Connections: []connectionView{}}
	byID := map[string]connectionView{}
	if out.Running {
		if st, err := a.host.ConnectStatus(r.Context(), x.t.ID, x.ws.ID); err == nil {
			out.CheckedAt = st.CheckedAt
			for _, p := range st.Providers {
				v := connectionView{Status: "not_connected", Method: p.Method, ExpiresAt: p.ExpiresAt}
				if p.Connected {
					v.Status = expiryStatus(p.ExpiresAt)
				}
				byID[p.Provider] = v
			}
		}
	}
	stored := a.storedKeys(r.Context(), x.t.ID, x.ws.ID)
	for _, p := range a.cfg.Connect.Providers {
		v, seen := byID[p.ID]
		if !seen {
			v.Status = "not_connected"
		}
		p.VaultixKey, v.ConnectProvider, v.Stored = "", p, stored[p.VaultixKey]
		out.Connections = append(out.Connections, v)
	}
	writeJSON(w, out)
}

// expiringWindow: a token within this of its expiry is shown as "expires in".
const expiringWindow = 7 * 24 * time.Hour

func expiryStatus(at *time.Time) string {
	if at != nil && time.Until(*at) < expiringWindow {
		return "expiring"
	}
	return "connected"
}

func (a *API) startConnectLogin(w http.ResponseWriter, r *http.Request) {
	x, ok := a.connectFor(w, r, true)
	if !ok || !a.connectAudit(r, x, "connect.login") {
		refuseIf(w, ok)
		return
	}
	st, err := a.host.StartLogin(r.Context(), x.t.ID, x.ws.ID, x.provider)
	hostAnswer(w, st, err, "The sign-in didn't start. Is the workspace running?")
}

func (a *API) connectLoginState(w http.ResponseWriter, r *http.Request) {
	x, ok := a.connectFor(w, r, true)
	if !ok {
		return
	}
	st, err := a.host.Login(r.Context(), x.t.ID, x.ws.ID, x.provider)
	hostAnswer(w, st, err, "No sign-in is in progress.")
}

func (a *API) submitConnectCode(w http.ResponseWriter, r *http.Request) {
	x, ok := a.connectFor(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Invalid request body.")
		return
	}
	err := a.host.SubmitLoginCode(r.Context(), x.t.ID, x.ws.ID, x.provider, in.Code)
	hostAnswer(w, map[string]string{"status": "ok"}, err, "The code wasn't accepted.")
}

func refuseIf(w http.ResponseWriter, resolved bool) {
	if resolved {
		refuseUnaudited(w)
	}
}

func hostAnswer(w http.ResponseWriter, v any, err error, msg string) {
	if err != nil {
		auth.Problem(w, http.StatusConflict, msg)
		return
	}
	writeJSON(w, v)
}
