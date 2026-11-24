package gh

import (
	"context"
	"errors"
	"net/url"
)

// ErrUserAccount: an installation token can't create repositories on a personal account (GitHub
// only allows that with a user token); the caller links an existing empty repo instead.
var ErrUserAccount = errors.New("github app installations can create repositories only in organizations")

// CreateOrgRepo creates a private, empty repository name in org through installation inst
// (needs the App's Administration: write permission on that org).
func (a *App) CreateOrgRepo(ctx context.Context, inst int64, org, accountType, name string) (*Repo, error) {
	if accountType != "Organization" {
		return nil, ErrUserAccount
	}
	t, err := a.InstallationToken(ctx, inst, nil, map[string]string{"administration": "write"})
	if err != nil {
		return nil, err
	}
	var r Repo
	in := map[string]any{"name": name, "private": true, "auto_init": false, "description": "Created by Shippin from a devspace folder"}
	if err := a.call(ctx, "POST", a.API+"/orgs/"+url.PathEscape(org)+"/repos", "token "+t.Value, in, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// PushToken is a token for one repository with contents write (push) or read (clone).
func (a *App) PushToken(ctx context.Context, inst, repoID int64, write bool) (Token, error) {
	p := "read"
	if write {
		p = "write"
	}
	return a.InstallationToken(ctx, inst, []int64{repoID}, map[string]string{"contents": p})
}
