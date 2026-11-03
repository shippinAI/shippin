package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/servers"
)

// probeTimeout bounds one hypervisor's share of the overview, so a dead host cannot hold the page.
const probeTimeout = 5 * time.Second

const (
	uiSettingKey      = "ui.config"
	hvBoardSettingKey = "approval.hv_board"
	sourceAPI         = "api"
	sourceAgent       = "agent"
)

var (
	hostSlugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	hostSiteRE = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
)

func uiKeyDef() platformKey {
	return platformKey{Key: uiSettingKey, Kind: "json", Label: "Interface",
		Help: "Names, visibility and order of navigation, providers and panel text.",
		def:  func(a *API) any { return a.cfg.UI },
		valid: func(v any) error {
			_, err := parseUI(v)
			return err
		}}
}

func hvBoardKeyDef() platformKey {
	return platformKey{Key: hvBoardSettingKey, Kind: "bool", Label: "Board approval for hypervisor changes",
		Help: "Start, stop, snapshot and other guest changes wait for a factory-board card. Turning this off makes them apply directly; each one is still audited.",
		def:  func(a *API) any { return a.cfg.Approval.NeedsBoard(hv.ActionLifecycle, "") }}
}

// parseUI accepts only the documented shape, so a typo is refused rather than silently ignored.
func parseUI(v any) (config.UI, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return config.UI{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var ui config.UI
	if err := dec.Decode(&ui); err != nil {
		return config.UI{}, fmt.Errorf("unknown or mistyped field: %v", err)
	}
	return ui, ui.Check()
}

// uiConfig is the config file with the ops override merged over it. A stored override that no
// longer validates is ignored rather than breaking the panel.
func (a *API) uiConfig(ctx context.Context) config.UI {
	if a.db == nil {
		return a.cfg.UI
	}
	m, err := a.db.PlatformSettings(ctx)
	if err != nil {
		log.Printf("ui settings: %v", err)
		return a.cfg.UI
	}
	raw, ok := m[uiSettingKey]
	if !ok {
		return a.cfg.UI
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return a.cfg.UI
	}
	over, err := parseUI(v)
	if err != nil {
		log.Printf("ui settings override ignored: %v", err)
		return a.cfg.UI
	}
	return a.cfg.UI.Merge(over)
}

// uiSettings is what the panel needs to draw itself. Drivers lets the Interface page offer a field
// for every provider, including ones the config file does not mention yet.
func (a *API) uiSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, struct {
		config.UI
		Drivers []hv.Spec `json:"drivers"`
	}{a.uiConfig(r.Context()), hv.Specs()})
}

// hvNeedsBoard is the approval rule, which the panel can switch off but not on: the config file
// stays the strictest word.
func (a *API) hvNeedsBoard(ctx context.Context, action, server string) bool {
	if !a.cfg.Approval.NeedsBoard(action, server) {
		return false
	}
	if a.db == nil {
		return true
	}
	on, ok := a.platformValue(ctx, hvBoardSettingKey).(bool)
	return !ok || on
}

type infraField struct {
	hv.Field
	Label       string `json:"label"`
	Help        string `json:"help,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Default     string `json:"default,omitempty"`
}

type infraGuestCount struct {
	Total   int `json:"total"`
	Running int `json:"running"`
}

// infraNode is one host under a provider. Online is true only when its API answered just now.
type infraNode struct {
	ID      string          `json:"id"`
	Slug    string          `json:"slug,omitempty"`
	Label   string          `json:"label"`
	Site    string          `json:"site,omitempty"`
	Kind    string          `json:"kind"`
	Source  string          `json:"source"`
	BaseURL string          `json:"base_url,omitempty"`
	Scope   string          `json:"scope,omitempty"`
	Online  bool            `json:"online"`
	Version string          `json:"version,omitempty"`
	Error   string          `json:"error,omitempty"`
	Guests  infraGuestCount `json:"guests"`
	FleetID string          `json:"fleet_id,omitempty"`
	Details *hv.NodeDetails `json:"details,omitempty"`
}

type infraTotals struct {
	Nodes   int `json:"nodes"`
	Online  int `json:"online"`
	Guests  int `json:"guests"`
	Running int `json:"running"`
}

type infraProvider struct {
	Kind         string       `json:"kind"`
	Label        string       `json:"label"`
	Description  string       `json:"description,omitempty"`
	Attachable   bool         `json:"attachable"`
	Capabilities []string     `json:"capabilities"`
	Verbs        []string     `json:"verbs"`
	Fields       []infraField `json:"fields"`
	Nodes        []infraNode  `json:"nodes"`
	Totals       infraTotals  `json:"totals"`
}

// infraAgent reports the local agent that is not answering, so it is not mistaken for a host.
type infraAgent struct {
	Label string `json:"label"`
	Error string `json:"error"`
}

// describeProvider joins what the driver can do with how this installation names it.
func describeProvider(spec hv.Spec, ui config.UI) (infraProvider, bool) {
	p := infraProvider{Kind: spec.Kind, Label: spec.Kind, Attachable: len(spec.Fields) > 0,
		Capabilities: spec.Capabilities, Verbs: spec.Verbs, Fields: []infraField{}, Nodes: []infraNode{}}
	cfg, _ := ui.Provider(spec.Kind)
	if cfg.Label != "" {
		p.Label = cfg.Label
	}
	p.Description = cfg.Description
	for _, f := range spec.Fields {
		out := infraField{Field: f, Label: f.Name}
		if i := slices.IndexFunc(cfg.Fields, func(c config.UIField) bool { return c.Name == f.Name }); i >= 0 {
			c := cfg.Fields[i]
			if c.Label != "" {
				out.Label = c.Label
			}
			out.Help, out.Placeholder, out.Default = c.Help, c.Placeholder, c.Default
		}
		p.Fields = append(p.Fields, out)
	}
	return p, !cfg.Hidden
}

// infraOverview is every provider with the hosts attached to it, read live. A host that does not
// answer stays listed with its error; it is never dropped and never reported as online.
func (a *API) infraOverview(w http.ResponseWriter, r *http.Request, _ *caller) {
	eps, err := a.hvDir().Endpoints(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Servers are unavailable right now.")
		return
	}
	refs, err := a.hvDir().Registry(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Servers are unavailable right now.")
		return
	}
	nodes := make([]infraNode, len(eps))
	var wg sync.WaitGroup
	for i, ep := range eps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), probeTimeout)
			defer cancel()
			nodes[i] = a.probeNode(ctx, ep)
		}()
	}
	wg.Wait()

	ui := a.uiConfig(r.Context())
	providers, order := []infraProvider{}, map[string]int{}
	for _, spec := range hv.Specs() {
		p, visible := describeProvider(spec, ui)
		if !visible {
			continue
		}
		order[p.Kind] = len(providers)
		providers = append(providers, p)
	}
	// A row of a kind with no driver spec, such as bare, is still reported under its own name.
	add := func(n infraNode) {
		i, ok := order[n.Kind]
		if !ok {
			p, _ := describeProvider(hv.Spec{Kind: n.Kind, Capabilities: []string{}, Verbs: []string{}}, ui)
			i, order[n.Kind] = len(providers), len(providers)
			providers = append(providers, p)
		}
		providers[i].Nodes = append(providers[i].Nodes, n)
	}
	for _, n := range nodes {
		add(n)
	}
	agentNode, agentDown := a.probeAgent(r.Context())
	if agentNode != nil {
		add(*agentNode)
	}

	total := infraTotals{}
	for i := range providers {
		for _, n := range providers[i].Nodes {
			providers[i].Totals.add(n)
			total.add(n)
		}
	}
	unattached := []servers.Ref{}
	for _, ref := range refs {
		if ref.Kind == "" {
			unattached = append(unattached, ref)
		}
	}
	fleets, err := a.hvDir().Fleets(r.Context())
	if err != nil || fleets == nil {
		fleets = []servers.Fleet{}
	}
	out := map[string]any{"providers": providers, "totals": total, "unattached": unattached, "fleets": fleets,
		"default_site": ui.DefaultSite, "checked_at": time.Now().UTC()}
	if agentDown != nil {
		out["local_agent"] = agentDown
	}
	writeJSON(w, out)
}

func (t *infraTotals) add(n infraNode) {
	t.Nodes++
	t.Guests += n.Guests.Total
	t.Running += n.Guests.Running
	if n.Online {
		t.Online++
	}
}

func (a *API) probeNode(ctx context.Context, ep servers.Endpoint) infraNode {
	n := infraNode{ID: ep.ID, Slug: ep.Slug, Label: ep.Label, Site: ep.Site, Kind: ep.Kind, Source: sourceAPI, BaseURL: ep.BaseURL, Scope: ep.Node, FleetID: ep.FleetID}
	if n.Label == "" {
		n.Label = ep.Slug
	}
	d, err := a.openDriver(ep)
	if err != nil {
		n.Error = err.Error()
		return n
	}
	h, err := d.Health(ctx)
	if err != nil {
		n.Error = err.Error()
		return n
	}
	n.Online, n.Version = h.Online, h.Version
	guests, err := d.Guests(ctx)
	if err != nil {
		n.Error = err.Error()
		return n
	}
	n.Guests.Total = len(guests)
	for _, g := range guests {
		if strings.EqualFold(g.Status, "running") {
			n.Guests.Running++
		}
	}
	// Missing detail is not a host that is down: a token may lack the right to read it.
	if ins, ok := d.(hv.Inspector); ok {
		if details, err := ins.Details(ctx); err == nil {
			n.Details = &details
		}
	}
	return n
}

// probeAgent reads the Incus project behind the local agent. An agent that does not answer is
// not a host someone added: a fresh install has none, so it comes back only as a note.
func (a *API) probeAgent(ctx context.Context) (*infraNode, *infraAgent) {
	if a.host == nil || a.cfg.Incus.HostdURL == "" {
		return nil, nil
	}
	label := a.cfg.Incus.HostLabel
	if label == "" {
		label = a.cfg.Incus.Project
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	insts, err := a.instances(ctx)
	if err != nil {
		return nil, &infraAgent{Label: label, Error: "Not answering."}
	}
	n := &infraNode{ID: sourceAgent, Label: label, Kind: "incus", Source: sourceAgent, Scope: a.cfg.Incus.Project, Online: true}
	n.Guests.Total = len(insts)
	for _, i := range insts {
		if strings.EqualFold(i.Status, "running") {
			n.Guests.Running++
		}
	}
	return n, nil
}

type hostAdd struct {
	hvAttach
	Label string `json:"label"`
	Slug  string `json:"slug"`
	Site  string `json:"site"`
}

// infraAdd creates a host and attaches its API in one step, so adding a provider does not need a
// server row made first in some org. Nothing here dials the address; the overview reports reach.
func (a *API) infraAdd(w http.ResponseWriter, r *http.Request, c *caller) {
	var in hostAdd
	if !decodeStrict(w, r, &in) {
		return
	}
	in.Label = strings.TrimSpace(in.Label)
	if n := utf8.RuneCountInString(in.Label); n < 1 || n > 120 || strings.ContainsAny(in.Label, "\r\n\t") {
		auth.Problem(w, http.StatusBadRequest, "Give the host a name of 1 to 120 characters.")
		return
	}
	if !hostSlugRE.MatchString(in.Slug) {
		auth.Problem(w, http.StatusBadRequest, "The slug uses lowercase letters, digits and dashes.")
		return
	}
	if in.Site == "" {
		in.Site = a.uiConfig(r.Context()).DefaultSite
	}
	if in.Site == "" {
		in.Site = in.Slug
	}
	if !hostSiteRE.MatchString(in.Site) {
		auth.Problem(w, http.StatusBadRequest, "The site uses lowercase letters, digits and dashes.")
		return
	}
	ep := in.endpoint()
	ep.Slug, ep.Label, ep.Site = in.Slug, in.Label, in.Site
	if err := validateEndpoint(ep, in.hvAttach); err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.permitAttach(w, r, c, in.Slug) {
		return
	}
	id, err := uuid.NewV7()
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The host could not be saved.")
		return
	}
	ep.ID = id.String()
	ctx, cancel := context.WithTimeout(r.Context(), connectTimeout)
	defer cancel()
	if err := a.prepareEndpoint(ctx, &ep, in.hvAttach); err != nil {
		auth.Problem(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := a.hvDir().CreateEndpoint(r.Context(), ep, c.id.Sub)
	if errors.Is(err, servers.ErrSlugTaken) {
		auth.Problem(w, http.StatusConflict, "A host with that slug already exists.")
		return
	}
	if err != nil {
		infraErr(w, err)
		return
	}
	writeStatus(w, http.StatusCreated, out)
}

// infraDetach removes the API from a host. The server row and its guests are untouched.
func (a *API) infraDetach(w http.ResponseWriter, r *http.Request, c *caller) {
	id := r.PathValue("id")
	if _, err := a.hvDir().Endpoint(r.Context(), id); err != nil {
		infraErr(w, err)
		return
	}
	if !a.permitAttach(w, r, c, id) {
		return
	}
	if err := a.hvDir().SetEndpoint(r.Context(), servers.Endpoint{ID: id}); err != nil {
		infraErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
