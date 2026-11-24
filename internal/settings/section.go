// Package settings is every configuration section the panel edits (CFG-01, docs/settings.md). A
// section is declared once, in the package that uses it, as `var Settings = settings.Section{...}` in
// a file settings.go; cmd/mcplgen collects them into actions.Settings. The panel's saved value wins;
// the bootstrap file only seeds an installation that has not saved the section yet. Key fields are
// kept in the secret store (ports.Secrets) and never returned.
package settings

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"shippin.cloud/console/internal/platform/config"
)

// Kind is what a field holds, and so how the panel shows it.
type Kind string

const (
	Text     Kind = "text"      // one line
	LongText Kind = "long_text" // several lines
	URL      Kind = "url"       // an http or https address without credentials
	Bool     Kind = "bool"      // true or false
	Integer  Kind = "integer"   // a whole number between Min and Max
	Choice   Kind = "choice"    // one of Choices
	List     Kind = "list"      // a list of short strings
	Map      Kind = "map"       // short names to short values
	Key      Kind = "key"       // a secret, one line: kept in the secret store, never shown
	LongKey  Kind = "long_key"  // a secret over several lines, such as a PEM private key
)

// Option is one allowed value of a choice field and what the panel calls it.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Field is one input of a section. Name is its JSON name, the same as in the config file.
type Field struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Help    string   `json:"help,omitempty"`
	Kind    Kind     `json:"kind"`
	Min     int      `json:"min,omitempty"`
	Max     int      `json:"max,omitempty"`
	Choices []Option `json:"choices,omitempty"`
	// Optional allows an empty value.
	Optional bool `json:"optional,omitempty"`
	// Source names a panel list the form picks the value from, for a field that holds an ID ("fleets").
	Source string `json:"source,omitempty"`
	// Env is the environment variable a key was read from before it moved to the panel. It is still
	// read until the section is saved in the panel, and the form offers to move it into the store.
	Env string `json:"env,omitempty"`
	// Valid checks a new key's value before it is written (a PEM that parses); nil accepts any.
	Valid func(string) error `json:"-"`
}

// Secret reports whether the field is a key.
func (f Field) Secret() bool { return f.Kind == Key || f.Kind == LongKey }

// Values are a section's non-key fields as JSON decodes them: string, bool, float64, []any, map[string]any.
type Values map[string]any

// Checking is what a section's check sees besides its own values.
type Checking interface {
	// Has reports whether key field name has a value, saved or entered in this change.
	Has(name string) bool
	// Section is another section's current values, for a rule across sections.
	Section(key string) Values
}

// Section is one form of the panel's settings.
type Section struct {
	Key     string
	Title   string
	Summary string
	Fields  []Field
	// Seed is the section as the bootstrap file and environment give it: usually the config struct,
	// whose JSON names are the field names.
	Seed func(c *config.Config) any
	// SeedKeys are the keys the environment gives, by field name.
	SeedKeys func(c *config.Config) map[string]string
	// Check validates the whole section before it is saved.
	Check func(v Values, c Checking) error
}

// As decodes values into T, the config struct the section's users read.
func As[T any](v Values) (T, error) {
	var out T
	raw, err := json.Marshal(v)
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, err
}

// CheckAs is a Check that decodes the values into T and runs its own Check, so a section reuses the
// rules its config struct already has.
func CheckAs[T interface{ Check() error }]() func(Values, Checking) error {
	return func(v Values, _ Checking) error {
		t, err := As[T](v)
		if err != nil {
			return err
		}
		return t.Check()
	}
}

var (
	sectionKey = regexp.MustCompile(`^[a-z][a-z0-9_]{1,40}$`)
	fieldName  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	mapName    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
)

// Validate checks a section's own description, so a mistake in one fails generation's tests, not a save.
func (s Section) Validate() error {
	if !sectionKey.MatchString(s.Key) || s.Title == "" || s.Seed == nil || len(s.Fields) == 0 {
		return fmt.Errorf("section %q needs a key, a title, a seed and fields", s.Key)
	}
	seen := map[string]bool{}
	for _, f := range s.Fields {
		switch {
		case !fieldName.MatchString(f.Name) || seen[f.Name]:
			return fmt.Errorf("section %s: field %q is unnamed or repeated", s.Key, f.Name)
		case f.Label == "":
			return fmt.Errorf("section %s: field %s has no label", s.Key, f.Name)
		case f.Kind == Choice && len(f.Choices) == 0:
			return fmt.Errorf("section %s: choice %s has no choices", s.Key, f.Name)
		case f.Kind == Integer && f.Max != 0 && f.Max < f.Min:
			return fmt.Errorf("section %s: %s has Max below Min", s.Key, f.Name)
		}
		switch f.Kind {
		case Text, LongText, URL, Bool, Integer, Choice, List, Map, Key, LongKey:
		default:
			return fmt.Errorf("section %s: %s has unknown kind %q", s.Key, f.Name, f.Kind)
		}
		seen[f.Name] = true
	}
	return nil
}

// field is the named field, if the section has it.
func (s Section) field(name string) (Field, bool) {
	for _, f := range s.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// check is one value against its field: the right type, within bounds. It returns the value as stored.
func (f Field) check(raw json.RawMessage) (any, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("%s: not a value", f.Label)
	}
	fail := func(m string) (any, error) { return nil, fmt.Errorf("%s %s", f.Label, m) }
	switch f.Kind {
	case Text, LongText, URL, Choice:
		s, ok := v.(string)
		if !ok {
			return fail("must be text.")
		}
		s = strings.TrimSpace(s)
		limit := 1000
		if f.Kind == LongText {
			limit = 64 << 10
		}
		if f.Max > 0 {
			limit = f.Max
		}
		switch {
		case s == "" && !f.Optional:
			return fail("is required.")
		case len(s) > limit:
			return fail(fmt.Sprintf("is longer than %d characters.", limit))
		case f.Kind != LongText && strings.IndexFunc(s, unicode.IsControl) >= 0:
			return fail("must be one line without control characters.")
		}
		if s != "" && f.Kind == URL {
			u, err := url.Parse(s)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return fail("must be an http or https address without a password, query or fragment.")
			}
		}
		if f.Kind == Choice {
			for _, c := range f.Choices {
				if c.Value == s {
					return s, nil
				}
			}
			return fail("must be one of the listed choices.")
		}
		return s, nil
	case Bool:
		if _, ok := v.(bool); !ok {
			return fail("must be true or false.")
		}
		return v, nil
	case Integer:
		n, ok := v.(float64)
		if !ok || n != float64(int64(n)) {
			return fail("must be a whole number.")
		}
		if int(n) < f.Min || (f.Max != 0 && int(n) > f.Max) {
			if f.Max != 0 {
				return fail(fmt.Sprintf("must be between %d and %d.", f.Min, f.Max))
			}
			return fail(fmt.Sprintf("must be at least %d.", f.Min))
		}
		return n, nil
	case List:
		items, ok := v.([]any)
		if !ok || len(items) > 200 {
			return fail("must be a list of at most 200 entries.")
		}
		out := make([]any, 0, len(items))
		for _, it := range items {
			s, ok := it.(string)
			s = strings.TrimSpace(s)
			if !ok || s == "" || len(s) > 200 || strings.IndexFunc(s, unicode.IsControl) >= 0 {
				return fail("entries must be short lines of text.")
			}
			out = append(out, s)
		}
		return out, nil
	case Map:
		m, ok := v.(map[string]any)
		if !ok || len(m) > 100 {
			return fail("must be at most 100 names and values.")
		}
		for k, x := range m {
			s, ok := x.(string)
			if !mapName.MatchString(k) || !ok || len(s) > 200 || strings.IndexFunc(s, unicode.IsControl) >= 0 {
				return fail("names must be short identifiers and values short text.")
			}
		}
		return m, nil
	}
	return fail("is a key, saved separately.")
}
