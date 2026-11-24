package api

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"shippin.cloud/console/internal/integrations/gh"
	"shippin.cloud/console/internal/integrations/github_app"
	"shippin.cloud/console/internal/platform/config"
	"shippin.cloud/console/internal/platform/job_queue"
	"shippin.cloud/console/internal/runtime/runner"
	"shippin.cloud/console/internal/settings"
)

// The sections below are read where they are used, so a save in the panel takes effect without a
// restart (CFG-01, docs/settings.md). An API built without settings (most tests) reads the bootstrap
// file, as the panel does for a section that has never been saved.

// liveJobs is the job queue section in force. A database that can't be read gives the file's values.
func (a *API) liveJobs(ctx context.Context) config.Jobs {
	if a.settings == nil {
		return a.cfg.Jobs
	}
	j, _ := settings.Get[config.Jobs](ctx, a.settings, job_queue.Settings.Key)
	return j
}

// liveRunners is the build runners section in force.
func (a *API) liveRunners(ctx context.Context) config.Runners {
	if a.settings == nil {
		return a.cfg.Runners
	}
	r, _ := settings.Get[config.Runners](ctx, a.settings, runner.Settings.Key)
	return r
}

// liveSection is a section decoded into T and its version; ok is false when the database couldn't be
// read, so that a caller keeps what it runs rather than falling back to the file for a moment.
func liveSection[T any](ctx context.Context, s *settings.Store, key string) (t T, version int, ok bool) {
	v, version, err := s.Values(ctx, key)
	if err != nil {
		log.Printf("settings %s: %v (keeping what runs)", key, err)
		return t, 0, false
	}
	if t, err = settings.As[T](v); err != nil {
		log.Printf("settings %s: %v (keeping what runs)", key, err)
		return t, 0, false
	}
	return t, version, true
}

// jobsSection is the job queue section and whether it could be read.
func (a *API) jobsSection(ctx context.Context) (config.Jobs, bool) {
	if a.settings == nil {
		return a.cfg.Jobs, true
	}
	j, _, ok := liveSection[config.Jobs](ctx, a.settings, job_queue.Settings.Key)
	return j, ok
}

// githubSection is the GitHub App section without its keys, with the App's identifiers, its version
// (0 while it comes from the file) and whether it could be read.
func (a *API) githubSection(ctx context.Context) (config.GitHub, int, bool) {
	if a.settings == nil {
		g := a.cfg.GitHub
		g.ClientSecret, g.PrivateKey, g.WebhookSecret = "", "", ""
		return g, 0, true
	}
	s, version, ok := liveSection[github_app.App](ctx, a.settings, github_app.Settings.Key)
	g := s.GitHub
	g.AppID, g.ClientID = s.AppID, s.ClientID
	return g, version, ok
}

// githubKeys are the App's keys: from the secret store once the section is saved there, else the
// environment.
func (a *API) githubKeys(ctx context.Context) (secret, private, hook string, err error) {
	if a.settings == nil {
		g := a.cfg.GitHub
		return g.ClientSecret, g.PrivateKey, g.WebhookSecret, nil
	}
	read := func(name string) string {
		if err != nil {
			return ""
		}
		var v string
		v, err = a.settings.Key(ctx, github_app.Settings.Key, name)
		return v
	}
	secret, private, hook = read("client_secret"), read("private_key"), read("webhook_secret")
	return secret, private, hook, err
}

// gitHubApp is the App client built from the github section. It is kept while the section stays the
// same, so the installation tokens it caches survive, and built again when the section changes.
type gitHubApp struct {
	mu   sync.Mutex
	have bool
	from ghFrom
	// app is nil while GitHub is off: the section turned off, or a key or the client ID missing.
	app  *gh.App
	hook string        // the webhook secret the app was built with
	cfg  config.GitHub // the section it was built from, for while the database can't be read
	// tried is when a build that left GitHub off was tried; it is tried again after settings.Fresh.
	tried  time.Time
	logged *ghFrom
}

// ghFrom is what an App was built from: the section's version, and for a section still in the file
// (version 0, nothing to mark a change) a digest of its values and keys.
type ghFrom struct {
	version int
	digest  [32]byte
}

var errGitHubOff = errors.New("GitHub isn't turned on in this installation")

// github is the GitHub App in force and its section, the webhook secret included (never logged or
// returned). A nil App means GitHub is off: the section is off, or a key or the client ID is missing.
func (a *API) github(ctx context.Context) (*gh.App, config.GitHub) {
	g, version, ok := a.githubSection(ctx)
	c := &a.ghApp
	c.mu.Lock()
	defer c.mu.Unlock()
	if !ok {
		if !c.have {
			return nil, config.GitHub{}
		}
		g = c.cfg
		g.WebhookSecret = c.hook
		return c.app, g
	}
	if !g.Enabled {
		c.have, c.app, c.hook, c.logged = false, nil, "", nil
		return nil, g
	}
	from := ghFrom{version: version}
	var secret, private, hook string
	var kerr error
	if version == 0 {
		// The environment's keys: nothing is fetched to read them.
		secret, private, hook, kerr = a.githubKeys(ctx)
		h := sha256.New()
		fmt.Fprintf(h, "%#v\x00%s\x00%s\x00%s", g, secret, private, hook)
		copy(from.digest[:], h.Sum(nil))
	}
	if c.have && c.from == from && (c.app != nil || time.Since(c.tried) < settings.Fresh) {
		g.WebhookSecret = c.hook
		return c.app, g
	}
	if version != 0 {
		secret, private, hook, kerr = a.githubKeys(ctx)
	}
	app, err := buildGitHub(g, secret, private, hook, kerr)
	c.have, c.from, c.cfg = true, from, g
	if err != nil {
		c.app, c.hook, c.tried = nil, "", time.Now()
		if c.logged == nil || *c.logged != from {
			log.Printf("github: off: %v", err) // names what is missing, never a key's value
			c.logged = &from
		}
		return nil, g
	}
	if c.logged != nil || c.app == nil {
		log.Printf("github: App on (settings version %d)", version)
	}
	c.app, c.hook, c.logged = app, hook, nil
	g.WebhookSecret = hook
	return app, g
}

func buildGitHub(g config.GitHub, secret, private, hook string, kerr error) (*gh.App, error) {
	switch {
	case kerr != nil:
		return nil, kerr
	case g.ClientID == "" || secret == "" || private == "" || hook == "":
		return nil, errors.New("the client ID, client secret, private key or webhook secret is missing")
	}
	return gh.New(g.ClientID, secret, private, g.APIBase, g.WebBase)
}

// githubOn is the App and its section, or errGitHubOff.
func (a *API) githubOn(ctx context.Context) (*gh.App, config.GitHub, error) {
	app, g := a.github(ctx)
	if app == nil {
		return nil, g, errGitHubOff
	}
	return app, g, nil
}
