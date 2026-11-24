package gh

import (
	"context"
	"fmt"
)

// Repo is one repository an installation can access.
type Repo struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
}

// Repos lists up to max repositories of installation inst (metadata read only).
func (a *App) Repos(ctx context.Context, inst int64, max int) ([]Repo, error) {
	t, err := a.InstallationToken(ctx, inst, nil, map[string]string{"metadata": "read"})
	if err != nil {
		return nil, err
	}
	out := []Repo{}
	for page := 1; len(out) < max; page++ {
		var res struct {
			Repositories []Repo `json:"repositories"`
		}
		u := fmt.Sprintf("%s/installation/repositories?per_page=100&page=%d", a.API, page)
		if err := a.call(ctx, "GET", u, "token "+t.Value, nil, &res); err != nil {
			return nil, err
		}
		out = append(out, res.Repositories...)
		if len(res.Repositories) < 100 {
			break
		}
	}
	if len(out) > max {
		out = out[:max]
	}
	return out, nil
}

// Repo finds repoID among installation inst's repositories (nil when it can't see it).
func (a *App) Repo(ctx context.Context, inst, repoID int64, max int) (*Repo, error) {
	rs, err := a.Repos(ctx, inst, max)
	if err != nil {
		return nil, err
	}
	for i := range rs {
		if rs[i].ID == repoID {
			return &rs[i], nil
		}
	}
	return nil, nil
}

// Installations lists the App's installations (polling when webhooks can't arrive).
func (a *App) Installations(ctx context.Context) ([]Installation, error) {
	auth, err := a.appAuth()
	if err != nil {
		return nil, err
	}
	out := []Installation{}
	for page := 1; page <= 50; page++ {
		var res []Installation
		if err := a.call(ctx, "GET", fmt.Sprintf("%s/app/installations?per_page=100&page=%d", a.API, page), auth, nil, &res); err != nil {
			return nil, err
		}
		out = append(out, res...)
		if len(res) < 100 {
			break
		}
	}
	return out, nil
}
