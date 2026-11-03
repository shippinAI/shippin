package config

import (
	"errors"
	"fmt"
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
	// Images are what a guest can be made from, beyond what the host already has.
	Images []UIImage `json:"images,omitempty"`
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
	uiLabelMax = 60
	uiTextMax  = 400
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

func uiString(where, s string, limit int) error {
	if utf8.RuneCountInString(s) > limit {
		return fmt.Errorf("%s: longer than %d characters", where, limit)
	}
	if strings.ContainsAny(s, "\x00\r\n\t") {
		return fmt.Errorf("%s: control characters are not allowed", where)
	}
	return nil
}

// Merge returns u with over applied: text by key, navigation by id, providers by kind. Entries
// in over replace the matching entry whole.
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
	out.Providers = mergeBy(u.Providers, over.Providers, func(p UIProvider) string { return p.Kind })
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
