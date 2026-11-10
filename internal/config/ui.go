package config

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// UI is the human-facing configuration of the panel: names, visibility and order. The panel
// ships built-in defaults for navigation and text; everything here overrides them. An ops
// override (the "ui" platform setting) merges over this file. It holds no secrets.
type UI struct {
	Nav UINav `json:"nav"`
	// Text overrides one panel string by its key (web/src/lib/ui-config/text-keys.ts).
	Text map[string]string `json:"text,omitempty"`
	// Providers names each hypervisor driver and its attach fields. A driver with no entry
	// shows its kind as its name.
	Providers []UIProvider `json:"providers,omitempty"`
	// DefaultSite prefills the site of a host added from the panel.
	DefaultSite string `json:"default_site,omitempty"`
}

// UINav overrides navigation sections and items by id.
type UINav struct {
	Sections []UINavEntry `json:"sections,omitempty"`
	Items    []UINavEntry `json:"items,omitempty"`
}

// UINavEntry renames, hides or moves one navigation section or item. Order 0 keeps its place.
type UINavEntry struct {
	ID     string `json:"id"`
	Label  string `json:"label,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
	Order  int    `json:"order,omitempty"`
}

// UIProvider is the configured presentation of one driver kind.
type UIProvider struct {
	Kind        string    `json:"kind"`
	Label       string    `json:"label,omitempty"`
	Description string    `json:"description,omitempty"`
	Hidden      bool      `json:"hidden,omitempty"`
	Fields      []UIField `json:"fields,omitempty"`
	// Images are what a guest can be made from, beyond what the host already has. In an override, null
	// keeps the images from the file and an empty list removes them.
	Images []UIImage `json:"images"`
	// ImageRemotes are the servers those images come from. An image id "name:alias" asks the remote
	// with that name for alias. Null and empty behave as they do for Images.
	ImageRemotes []UIImageRemote `json:"image_remotes"`
}

// UIImageRemote is one server images are fetched from.
type UIImageRemote struct {
	Name     string `json:"name"`
	Server   string `json:"server"`
	Protocol string `json:"protocol"`
}

// ImageRemote finds the configured server an image id refers to, and the alias to ask it for.
func (u UI) ImageRemote(kind, image string) (UIImageRemote, string, bool) {
	name, alias, found := strings.Cut(image, ":")
	if !found || alias == "" {
		return UIImageRemote{}, "", false
	}
	p, ok := u.Provider(kind)
	if !ok {
		return UIImageRemote{}, "", false
	}
	for _, r := range p.ImageRemotes {
		if r.Name == name {
			return r, alias, true
		}
	}
	return UIImageRemote{}, "", false
}

// UIImage is one image this installation offers. The id is in the driver's own terms.
type UIImage struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind,omitempty"`
}

// UIField is the configured presentation of one attach field. Default prefills the form.
type UIField struct {
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`
	Help        string `json:"help,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Default     string `json:"default,omitempty"`
}

var (
	uiIDRE   = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z0-9_-]+)*$`)
	uiSiteRE = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
)

const (
	uiLabelMax   = 60
	uiTextMax    = 400
	uiImageIDMax = 200
	uiImagesMax  = 200
)

// Check bounds every value so a bad override cannot break the panel it would need to undo itself.
func (u UI) Check() error {
	if u.DefaultSite != "" && !uiSiteRE.MatchString(u.DefaultSite) {
		return errors.New("ui.default_site must be lowercase letters, digits and dashes")
	}
	for _, entries := range [][]UINavEntry{u.Nav.Sections, u.Nav.Items} {
		for _, e := range entries {
			if !uiIDRE.MatchString(e.ID) {
				return fmt.Errorf("ui.nav: %q is not a navigation id", e.ID)
			}
			if err := uiString(e.ID, e.Label, uiLabelMax); err != nil {
				return err
			}
		}
	}
	for k, v := range u.Text {
		if !uiIDRE.MatchString(k) {
			return fmt.Errorf("ui.text: %q is not a text key", k)
		}
		if err := uiString(k, v, uiTextMax); err != nil {
			return err
		}
	}
	for _, p := range u.Providers {
		if !uiIDRE.MatchString(p.Kind) {
			return fmt.Errorf("ui.providers: %q is not a driver kind", p.Kind)
		}
		if err := uiString(p.Kind, p.Label, uiLabelMax); err != nil {
			return err
		}
		if err := uiString(p.Kind, p.Description, uiTextMax); err != nil {
			return err
		}
		if err := validImages(p); err != nil {
			return err
		}
		for _, f := range p.Fields {
			if !uiIDRE.MatchString(f.Name) {
				return fmt.Errorf("ui.providers.%s: %q is not a field name", p.Kind, f.Name)
			}
			for _, s := range []string{f.Label, f.Help, f.Placeholder, f.Default} {
				if err := uiString(p.Kind+"."+f.Name, s, uiTextMax); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// validImages bounds a provider's images and image servers, whose names must be unique.
func validImages(p UIProvider) error {
	if len(p.Images) > uiImagesMax || len(p.ImageRemotes) > uiImagesMax {
		return fmt.Errorf("ui.providers.%s: at most %d images and %d image servers", p.Kind, uiImagesMax, uiImagesMax)
	}
	remotes := map[string]bool{}
	for _, r := range p.ImageRemotes {
		if err := validImageRemote(p.Kind, r); err != nil {
			return err
		}
		if remotes[r.Name] {
			return fmt.Errorf("ui.providers.%s: image server %q is listed twice", p.Kind, r.Name)
		}
		remotes[r.Name] = true
	}
	ids := map[string]bool{}
	for _, m := range p.Images {
		if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.Label) == "" {
			return fmt.Errorf("ui.providers.%s: every image needs an id and a label", p.Kind)
		}
		if err := uiString(p.Kind+" image id", m.ID, uiImageIDMax); err != nil {
			return err
		}
		if err := uiString(p.Kind+" image label", m.Label, uiLabelMax); err != nil {
			return err
		}
		if m.Kind != "" && m.Kind != "vm" && m.Kind != "ct" {
			return fmt.Errorf("ui.providers.%s: image %q has an unknown kind %q", p.Kind, m.ID, m.Kind)
		}
		if ids[m.ID] {
			return fmt.Errorf("ui.providers.%s: image %q is listed twice", p.Kind, m.ID)
		}
		ids[m.ID] = true
	}
	return nil
}

// validImageRemote keeps a remote to https and a known protocol. "local" already means the host itself.
func validImageRemote(kind string, r UIImageRemote) error {
	if !uiIDRE.MatchString(r.Name) || r.Name == "local" {
		return fmt.Errorf("ui.providers.%s: %q cannot name an image server", kind, r.Name)
	}
	u, err := url.Parse(r.Server)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("ui.providers.%s: image server %q must be an https URL", kind, r.Name)
	}
	if !slices.Contains([]string{"simplestreams", "incus", "oci"}, r.Protocol) {
		return fmt.Errorf("ui.providers.%s: image server %q has an unknown protocol %q", kind, r.Name, r.Protocol)
	}
	return nil
}

func uiString(where, s string, limit int) error {
	if utf8.RuneCountInString(s) > limit {
		return fmt.Errorf("%s: longer than %d characters", where, limit)
	}
	if strings.ContainsAny(s, "\x00\r\n\t") {
		return fmt.Errorf("%s: control characters are not allowed", where)
	}
	return nil
}

// Merge returns u with over applied: text by key, navigation by id, providers by kind. Entries in over
// replace the matching entry whole, except that a provider's images and image servers carry over when
// the override does not mention them.
func (u UI) Merge(over UI) UI {
	out := UI{DefaultSite: u.DefaultSite}
	if over.DefaultSite != "" {
		out.DefaultSite = over.DefaultSite
	}
	if len(u.Text)+len(over.Text) > 0 {
		out.Text = make(map[string]string, len(u.Text)+len(over.Text))
		for k, v := range u.Text {
			out.Text[k] = v
		}
		for k, v := range over.Text {
			out.Text[k] = v
		}
	}
	navID := func(e UINavEntry) string { return e.ID }
	out.Nav.Sections = mergeBy(u.Nav.Sections, over.Nav.Sections, navID)
	out.Nav.Items = mergeBy(u.Nav.Items, over.Nav.Items, navID)
	providerKind := func(p UIProvider) string { return p.Kind }
	providers := slices.Clone(over.Providers)
	for i, p := range providers {
		base, ok := u.Provider(p.Kind)
		if !ok {
			continue
		}
		if p.Images == nil {
			providers[i].Images = base.Images
		}
		if p.ImageRemotes == nil {
			providers[i].ImageRemotes = base.ImageRemotes
		}
	}
	out.Providers = mergeBy(u.Providers, providers, providerKind)
	return out
}

func mergeBy[T any](base, over []T, id func(T) string) []T {
	out := slices.Clone(base)
	for _, o := range over {
		if i := slices.IndexFunc(out, func(b T) bool { return id(b) == id(o) }); i >= 0 {
			out[i] = o
		} else {
			out = append(out, o)
		}
	}
	return out
}

// Provider returns the configured presentation of kind.
func (u UI) Provider(kind string) (UIProvider, bool) {
	i := slices.IndexFunc(u.Providers, func(p UIProvider) bool { return p.Kind == kind })
	if i < 0 {
		return UIProvider{}, false
	}
	return u.Providers[i], true
}
