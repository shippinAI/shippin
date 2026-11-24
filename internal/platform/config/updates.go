package config

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"time"
)

// Updates is where this installation looks for new builds. A fork points it at its own repository.
// The check is one read of a public API; nothing is downloaded or applied.
type Updates struct {
	Enabled         bool   `json:"enabled"`
	Repository      string `json:"repository"` // owner/name
	Branch          string `json:"branch"`
	IntervalMinutes int    `json:"interval_minutes"`
	APIURL          string `json:"api_url"`
	// Command overrides the update command the panel shows. Empty derives it from how it was deployed.
	Command string `json:"command,omitempty"`
}

var updatesRepoRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func (u *Updates) load() {
	if v := os.Getenv("SHIPPIN_UPDATE_ENABLED"); v != "" {
		u.Enabled = v == "1" || v == "true" || v == "yes"
	}
	if v := os.Getenv("SHIPPIN_UPDATE_REPOSITORY"); v != "" {
		u.Repository = v
	}
	if v := os.Getenv("SHIPPIN_UPDATE_BRANCH"); v != "" {
		u.Branch = v
	}
	if v := os.Getenv("SHIPPIN_UPDATE_API_URL"); v != "" {
		u.APIURL = v
	}
	if v := os.Getenv("SHIPPIN_UPDATE_COMMAND"); v != "" {
		u.Command = v
	}
}

// Check is fail-closed: a half-configured check is an error at start, not a silent no-op.
func (u Updates) Check() error {
	if !u.Enabled {
		return nil
	}
	if !updatesRepoRE.MatchString(u.Repository) {
		return errors.New("updates.repository must be owner/name")
	}
	if u.Branch == "" || strings.ContainsAny(u.Branch, " \t\r\n") {
		return errors.New("updates.branch is required")
	}
	if !strings.HasPrefix(u.APIURL, "https://") && !strings.HasPrefix(u.APIURL, "http://") {
		return errors.New("updates.api_url must be an http(s) URL")
	}
	return nil
}

// Interval is how long a result is kept (default 30 minutes: unauthenticated API reads are rationed).
func (u Updates) Interval() time.Duration { return minutesOr(u.IntervalMinutes, 30) }
