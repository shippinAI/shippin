package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"shippin.cloud/console/internal/auth"
)

// Platform settings: a closed list of config keys ops may override from the panel. The config
// file stays the default (a cleared override falls back to it); each write is audited in its tx.
type platformKey struct {
	Key, Label, Help string
	Kind             string            // "string" | "bool" | "json"
	def              func(a *API) any  // the config file value
	valid            func(v any) error // nil = ok
}

var accountPathRe = regexp.MustCompile(`^(/[A-Za-z0-9._~-]+)+/?$`)

func (a *API) platformKeys() []platformKey {
	return []platformKey{
		issuerKey(a.cfg),
		{Key: "idp.account_path", Kind: "string", Label: "IdP account page path",
			Help: "Path under the IdP issuer for the user's own account page; empty hides the link.",
			def:  func(a *API) any { return a.cfg.IDP.AccountPath },
			valid: func(v any) error {
				if s, _ := v.(string); s != "" && (len(s) > 200 || !accountPathRe.MatchString(s)) {
					return fmt.Errorf("must be a path like /ui/console/users/me")
				}
				return nil
			}},
		{Key: "services.rrflow.auto_attach", Kind: "bool", Label: "rrflow auto-attach on devspace creation",
			Help: "New project devspaces get a namespace in the tenant VM's rrflow (only where rrflow is enabled).",
			def:  func(a *API) any { return a.cfg.Services.RRFlow.AutoAttach }},
		uiKeyDef(),
		hvBoardKeyDef(),
	}
}

func (a *API) platformRoutes() []opRoute {
	return []opRoute{
		{"GET /v1/operator/platform-settings", "platform.settings.read", a.opPlatformSettings},
		{"PUT /v1/operator/platform-settings/{key}", "platform.settings.write", a.opSetPlatformSetting},
	}
}

// platformValue is the effective value of key: the ops override, else the config file.
func (a *API) platformValue(ctx context.Context, key string) any {
	for _, k := range a.platformKeys() {
		if k.Key != key {
			continue
		}
		if m, err := a.db.PlatformSettings(ctx); err == nil {
			if raw, ok := m[key]; ok {
				var v any
				if json.Unmarshal(raw, &v) == nil && (k.valid == nil || k.valid(v) == nil) {
					return v
				}
			}
		} else {
			log.Printf("platform settings: %v", err) // config value applies
		}
		return k.def(a)
	}
	return nil
}

// rrflowAutoAttach: auto-attach (ops override or config) and rrflow enabled for tenant.
func (a *API) rrflowAutoAttach(ctx context.Context, tenant string) bool {
	on, _ := a.platformValue(ctx, "services.rrflow.auto_attach").(bool)
	return on && a.cfg.Services.RRFlow.Allows(tenant)
}

func (a *API) opPlatformSettings(w http.ResponseWriter, r *http.Request, _ *caller) {
	m, _ := a.db.PlatformSettings(r.Context())
	out := []map[string]any{}
	for _, k := range a.platformKeys() {
		_, over := m[k.Key]
		out = append(out, map[string]any{"key": k.Key, "label": k.Label, "help": k.Help, "kind": k.Kind,
			"config_value": k.def(a), "value": a.platformValue(r.Context(), k.Key), "overridden": over})
	}
	writeJSON(w, map[string]any{"items": out})
}

func (a *API) opSetPlatformSetting(w http.ResponseWriter, r *http.Request, c *caller) {
	var k *platformKey
	for _, x := range a.platformKeys() {
		if x.Key == r.PathValue("key") {
			k = &x
		}
	}
	var in struct {
		Value  json.RawMessage `json:"value"` // null clears the override
		Reason string          `json:"reason"`
	}
	if k == nil {
		auth.Problem(w, http.StatusNotFound, "Unknown setting.")
		return
	}
	if !decodeStrict(w, r, &in) || !reasonOK(w, in.Reason) {
		return
	}
	var v any
	if in.Value != nil && string(in.Value) != "null" {
		_ = json.Unmarshal(in.Value, &v)
		_, isS := v.(string)
		_, isB := v.(bool)
		_, isO := v.(map[string]any)
		if (k.Kind == "string" && !isS) || (k.Kind == "bool" && !isB) || (k.Kind == "json" && !isO) {
			auth.Problem(w, http.StatusBadRequest, "The value must be a "+k.Kind+".")
			return
		}
		if k.valid != nil {
			if err := k.valid(v); err != nil {
				auth.Problem(w, http.StatusBadRequest, k.Label+": "+err.Error())
				return
			}
		}
	} else {
		in.Value = nil
	}
	old := a.platformValue(r.Context(), k.Key)
	detail := fmt.Sprintf("%s: %v → %s · %s", k.Key, old, strings.TrimSpace(string(in.Value)), in.Reason)
	if k.Kind == "json" { // the previous document is too long to read in an audit line
		detail = fmt.Sprintf("%s → %s · %s", k.Key, strings.TrimSpace(string(in.Value)), in.Reason)
	}
	e := teamEntry(r, c, "platform.setting", detail)
	if err := a.db.SetPlatformSetting(r.Context(), k.Key, in.Value, c.id.Sub, e); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The setting couldn't be saved.")
		return
	}
	writeJSON(w, map[string]any{"key": k.Key, "value": a.platformValue(r.Context(), k.Key)})
}
