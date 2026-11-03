package api

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"shippin.cloud/console/internal/config"
)

// accessIn is the body of an access request or a break-glass activation.
type accessIn struct {
	TenantID      string `json:"tenant_id"`
	WorkspaceID   string `json:"workspace_id"`
	Reason        string `json:"reason"`
	Duration      string `json:"duration"`
	Category      string `json:"category"`
	Justification string `json:"justification"`
	Withhold      bool   `json:"withhold_notice"`
}

// checkText bounds free text (reason/justification) and refuses control characters other than
// newlines and tabs, so audit views can't be spoofed with terminal or bidi tricks.
func checkText(s, what string, min, max int) string {
	n := utf8.RuneCountInString(s)
	if n < min || n > max {
		return what + " must be " + itoa(min) + " to " + itoa(max) + " characters."
	}
	for _, r := range s {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.Is(unicode.Bidi_Control, r) {
			return what + " contains characters that aren't allowed."
		}
	}
	return ""
}

// checkRequest validates a consent request; the duration must be one of the configured choices.
func checkRequest(cfg config.Access, in *accessIn) (time.Duration, string) {
	in.Reason = strings.TrimSpace(in.Reason)
	if !uuidRe.MatchString(in.TenantID) || (in.WorkspaceID != "" && !uuidRe.MatchString(in.WorkspaceID)) {
		return 0, "Choose a tenant (and optionally one of its workspaces)."
	}
	if msg := checkText(in.Reason, "The reason", cfg.ReasonMin, cfg.ReasonMax); msg != "" {
		return 0, msg
	}
	for i, s := range cfg.Durations {
		if s == in.Duration {
			return cfg.DurationList()[i], ""
		}
	}
	return 0, "Choose one of the offered durations: " + strings.Join(cfg.Durations, ", ") + "."
}

// checkBreakGlass validates an activation. reauth is true when the admin must sign in again
// first (no IdP login within reauth_minutes; bearer tokens never qualify).
func checkBreakGlass(cfg config.Access, in *accessIn, authAt, now time.Time) (cat config.Category, msg string, reauth bool) {
	b := cfg.BreakGlass
	in.Justification = strings.TrimSpace(in.Justification)
	if !uuidRe.MatchString(in.TenantID) || (in.WorkspaceID != "" && !uuidRe.MatchString(in.WorkspaceID)) {
		return cat, "Choose a tenant (and optionally one of its workspaces).", false
	}
	cat, ok := b.Category(in.Category)
	if !ok {
		return cat, "Choose a break-glass category.", false
	}
	if m := checkText(in.Justification, "The justification", b.JustificationMin, cfg.ReasonMax); m != "" {
		return cat, m, false
	}
	if in.Withhold && !cat.WithholdNotice {
		return cat, "Only the " + withholdLabels(b) + " category may delay the customer notice.", false
	}
	if authAt.IsZero() || now.Sub(authAt) > time.Duration(b.ReauthMinutes)*time.Minute || authAt.After(now.Add(time.Minute)) {
		return cat, "Sign in again to confirm it's you (within the last " + itoa(b.ReauthMinutes) + " minutes).", true
	}
	return cat, "", false
}

// noticeAt is when the customer is told: now, unless a withhold category asked to delay it.
func noticeAt(cat config.Category, withhold bool, now time.Time) time.Time {
	if withhold && cat.WithholdNotice {
		if d, err := time.ParseDuration(cat.NoticeDelay); err == nil && d > 0 {
			return now.Add(d)
		}
	}
	return now
}

func withholdLabels(b config.BreakGlass) string {
	out := []string{}
	for _, c := range b.Categories {
		if c.WithholdNotice {
			out = append(out, c.Label)
		}
	}
	return strings.Join(out, " / ")
}
