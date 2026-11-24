package config

import (
	"errors"
	"time"
)

// Access configures customer-granted platform access and break-glass.
type Access struct {
	// Durations are the choices an admin may request (Go durations, e.g. "1h").
	Durations []string `json:"durations"`
	ReasonMin int      `json:"reason_min"`
	ReasonMax int      `json:"reason_max"`
	// SweepSeconds is how often expired grants are closed and audited.
	SweepSeconds int `json:"sweep_seconds"`
	// LogWindow dedupes access entries per admin and host (every open is still logged once).
	LogWindow  string     `json:"log_window"`
	BreakGlass BreakGlass `json:"break_glass"`
}

// BreakGlass is emergency access without customer approval, heavily audited.
type BreakGlass struct {
	Duration string `json:"duration"`
	// ReauthMinutes: the admin must have signed in this recently (fresh IdP login).
	ReauthMinutes    int        `json:"reauth_minutes"`
	JustificationMin int        `json:"justification_min"`
	Categories       []Category `json:"categories"`
	// NoticeHours: how long the customer banner shows after the notice becomes visible.
	NoticeHours int `json:"notice_hours"`
}

// Category is a break-glass reason. Only a category with WithholdNotice may delay the
// customer notice, by NoticeDelay (the notice is never suppressed for good).
type Category struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	WithholdNotice bool   `json:"withhold_notice,omitempty"`
	NoticeDelay    string `json:"notice_delay,omitempty"`
}

const (
	maxGrant  = 7 * 24 * time.Hour
	maxNotice = 90 * 24 * time.Hour
)

// DurationList parses Durations (Check guarantees they parse).
func (a Access) DurationList() []time.Duration {
	out := []time.Duration{}
	for _, s := range a.Durations {
		if d, err := time.ParseDuration(s); err == nil {
			out = append(out, d)
		}
	}
	return out
}

// Category returns the configured break-glass category with id.
func (b BreakGlass) Category(id string) (Category, bool) {
	for _, c := range b.Categories {
		if c.ID == id {
			return c, true
		}
	}
	return Category{}, false
}

func span(s string) (time.Duration, bool) {
	d, err := time.ParseDuration(s)
	return d, err == nil && d > 0 && d <= maxGrant
}

// Check fails closed on any missing or unbounded access setting.
func (a Access) Check() error {
	if len(a.Durations) == 0 || a.ReasonMin < 1 || a.ReasonMax < a.ReasonMin || a.SweepSeconds < 1 {
		return errors.New("access.durations, reason_min/max and sweep_seconds are required")
	}
	for _, s := range append([]string{a.LogWindow, a.BreakGlass.Duration}, a.Durations...) {
		if _, ok := span(s); !ok {
			return errors.New("access: durations must be positive and at most 7 days: " + s)
		}
	}
	b := a.BreakGlass
	if b.ReauthMinutes < 1 || b.JustificationMin < 1 || b.NoticeHours < 1 || len(b.Categories) == 0 {
		return errors.New("access.break_glass needs reauth_minutes, justification_min, notice_hours and categories")
	}
	seen := map[string]bool{}
	for _, c := range b.Categories {
		if c.ID == "" || c.Label == "" || seen[c.ID] {
			return errors.New("access.break_glass.categories need unique id and label")
		}
		seen[c.ID] = true
		if (c.NoticeDelay != "") != c.WithholdNotice {
			return errors.New("notice_delay is required on, and only allowed on, a withhold_notice category")
		}
		if d, err := time.ParseDuration(c.NoticeDelay); c.NoticeDelay != "" && (err != nil || d <= 0 || d > maxNotice) {
			return errors.New("access.break_glass notice_delay is invalid: " + c.NoticeDelay)
		}
	}
	return nil
}
