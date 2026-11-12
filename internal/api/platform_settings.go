package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
)

// Platform settings: a closed list of config keys ops may override from the panel. The config
// file stays the default (a cleared override falls back to it); each write is audited in its tx.
type platformKey struct {
	Key, Label, Help string
	Kind             string            // "string" | "bool" | "integer" | "choice" | "json" | "array"
	Min, Max         int               // integer bounds exposed to the existing settings UI
	Choices          []settingChoice   // the allowed values of a "choice" key, in display order
	def              func(a *API) any  // the config file value
	valid            func(v any) error // nil = ok
}

// settingChoice is one allowed value of a "choice" setting and what the panel calls it.
type settingChoice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// signupChoices are the sign-up modes, most closed first.
var signupChoices = []settingChoice{
	{"invite_only", "Invitation only"},
	{"waitlist", "Invitation only, with a public waitlist"},
	{"open", "Anyone can sign up"},
}

func oneOfChoices(cs []settingChoice) func(any) error {
	return func(v any) error {
		s, _ := v.(string)
		for _, c := range cs {
			if c.Value == s {
				return nil
			}
		}
		return fmt.Errorf("must be one of the listed choices")
	}
}

var accountPathRe = regexp.MustCompile(`^(/[A-Za-z0-9._~-]+)+/?$`)

func validateProbes(value any) error {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 32<<10 {
		return fmt.Errorf("probe configuration exceeds the 32 KiB limit")
	}
	var probes []config.Probe
	if err := json.Unmarshal(raw, &probes); err != nil {
		return fmt.Errorf("must be a list of probe definitions")
	}
	if len(probes) > 50 {
		return fmt.Errorf("at most 50 probes are allowed")
	}
	seen := map[string]bool{}
	for _, p := range probes {
		name := strings.TrimSpace(p.Name)
		if name == "" || len(name) > 80 || p.Kind == "" || len(p.Kind) > 40 {
			return fmt.Errorf("each probe needs a name and kind")
		}
		if seen[strings.ToLower(name)] {
			return fmt.Errorf("probe names must be unique")
		}
		seen[strings.ToLower(name)] = true
		if (p.URL == "") == (p.TCP == "") {
			return fmt.Errorf("%s must have exactly one HTTP URL or TCP address", name)
		}
		if p.URL != "" {
			u, err := url.ParseRequestURI(p.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
				return fmt.Errorf("%s needs an HTTP or HTTPS URL without credentials, query or fragment", name)
			}
		}
		if p.TCP != "" {
			if _, _, err := net.SplitHostPort(p.TCP); err != nil {
				return fmt.Errorf("%s needs a host:port TCP address", name)
			}
		}
		if strings.ContainsAny(p.Host, "\r\n") {
			return fmt.Errorf("%s has an invalid Host header", name)
		}
		if p.TimeoutSeconds < 0 || p.TimeoutSeconds > 10 {
			return fmt.Errorf("%s timeout must be between 1 and 10 seconds when set", name)
		}
		if (p.ServerID == "") != (p.GuestID == "") || len(p.GuestID) > 128 || strings.IndexFunc(p.GuestID, unicode.IsControl) >= 0 {
			return fmt.Errorf("%s provider resource needs a server ID and guest ID without control characters", name)
		}
		if p.ServerID != "" {
			if _, err := uuid.Parse(p.ServerID); err != nil {
				return fmt.Errorf("%s provider resource needs a valid system UUID", name)
			}
		}
	}
	return nil
}

func (a *API) platformKeys() []platformKey {
	keys := []platformKey{
		{Key: "services.probe_concurrency", Kind: "integer", Min: 1, Max: config.MaxProbeConcurrency, Label: "Concurrent health checks",
			Help: fmt.Sprintf("Maximum simultaneous scheduled, manual HTTP/TCP and provider-resource checks across this console process (1–%d).", config.MaxProbeConcurrency),
			def:  func(a *API) any { return a.cfg.ProbeConcurrency }, valid: func(v any) error {
				n, ok := v.(float64)
				if !ok || n != float64(int(n)) || n < 1 || n > config.MaxProbeConcurrency {
					return fmt.Errorf("must be a whole number between 1 and %d", config.MaxProbeConcurrency)
				}
				return nil
			}},
		{Key: "services.observation_retention_days", Kind: "integer", Min: 1, Max: 365, Label: "Health observation retention (days)",
			Help: "How long platform health observations remain available (1 to 365 days; default 30).",
			def:  func(*API) any { return 30 }, valid: func(v any) error {
				n, ok := v.(float64)
				if !ok || n != float64(int(n)) || n < 1 || n > 365 {
					return fmt.Errorf("must be a whole number between 1 and 365")
				}
				return nil
			}},
		{Key: "infra.power_review_ttl_minutes", Kind: "integer", Min: 1, Max: 1440, Label: "Guest power review window (minutes)",
			Help: "How long Shippin accepts an AgentHub guest power review before requiring a new review (1 to 1440 minutes; default 30). This does not expire or close the AgentHub ticket.",
			def:  func(*API) any { return float64(30) }, valid: func(v any) error {
				n, ok := v.(float64)
				if !ok || n != float64(int(n)) || n < 1 || n > 1440 {
					return fmt.Errorf("must be a whole number between 1 and 1440")
				}
				return nil
			}},
		uiKeyDef(),
		{Key: "directory.signup", Kind: "choice", Choices: signupChoices, Label: "Who can sign up",
			Help: "Invitation only: people need an invitation. With a waitlist: the sign-in page also lets anyone ask to join, and you invite them from the list. Anyone: new accounts are created on first sign-in. Takes effect on the next sign-in.",
			def:  func(a *API) any { return a.cfg.Directory.Signup }, valid: oneOfChoices(signupChoices)},
	}
	if a.cfg.IDP.ClientID != "" {
		keys = append(keys, issuerKey(a.cfg), platformKey{Key: "idp.account_path", Kind: "string", Label: "IdP account page path",
			Help: "Path under the IdP issuer for the user's own account page; empty hides the link.",
			def:  func(a *API) any { return a.cfg.IDP.AccountPath },
			valid: func(v any) error {
				if s, _ := v.(string); s != "" && (len(s) > 200 || !accountPathRe.MatchString(s)) {
					return fmt.Errorf("must be a path like /ui/console/users/me")
				}
				return nil
			}})
	}
	if a.cfg.Services.RRFlow.Enabled {
		keys = append(keys, platformKey{Key: "services.rrflow.auto_attach", Kind: "bool", Label: "rrflow auto-attach on devspace creation",
			Help: "New project devspaces get a namespace in the tenant VM's rrflow (only where rrflow is enabled).",
			def:  func(a *API) any { return a.cfg.Services.RRFlow.AutoAttach }})
	}
	return keys
}

func (a *API) platformRoutes() []opRoute {
	return append([]opRoute{
		{"GET /v1/operator/platform-settings", "platform.settings.read", a.opPlatformSettings},
		{"PUT /v1/operator/platform-settings/{key}", "platform.settings.write", a.opSetPlatformSetting},
		{"GET /v1/operator/agenthub-connection", "platform.settings.read", a.opAgentHubConnection},
		{"PUT /v1/operator/agenthub-connection", "platform.settings.write", a.opSetAgentHubConnection},
		{"POST /v1/operator/agenthub-connection/test", "platform.settings.write", a.opTestAgentHubConnection},
	}, append(a.platformServiceRoutes(), a.platformContextRoutes()...)...)
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

// signupMode is the sign-up mode in force: the panel's setting, else the config file.
func (a *API) signupMode(ctx context.Context) string {
	if a.db == nil {
		return a.cfg.Directory.Signup
	}
	m, _ := a.platformValue(ctx, "directory.signup").(string)
	return m
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
		row := map[string]any{"key": k.Key, "label": k.Label, "help": k.Help, "kind": k.Kind, "min": k.Min, "max": k.Max,
			"config_value": k.def(a), "value": a.platformValue(r.Context(), k.Key), "overridden": over}
		if k.Choices != nil {
			row["choices"] = k.Choices
		}
		out = append(out, row)
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
		_, isA := v.([]any)
		_, isN := v.(float64)
		if ((k.Kind == "string" || k.Kind == "choice") && !isS) || (k.Kind == "bool" && !isB) || (k.Kind == "json" && !isO) || (k.Kind == "array" && !isA) || (k.Kind == "integer" && !isN) {
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
	if k.Kind == "json" || k.Kind == "array" {
		detail = fmt.Sprintf("%s updated (configuration values redacted) · %s", k.Key, in.Reason)
		if in.Value == nil {
			detail = fmt.Sprintf("%s reset to config (configuration values redacted) · %s", k.Key, in.Reason)
		}
	}
	e := teamEntry(r, c, "platform.setting", detail)
	if err := a.db.SetPlatformSetting(r.Context(), k.Key, in.Value, c.id.Sub, e); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The setting couldn't be saved.")
		return
	}
	writeJSON(w, map[string]any{"key": k.Key, "value": a.platformValue(r.Context(), k.Key)})
}
