package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ErrNotYours: the installation id from the setup redirect is not one the signing-in GitHub user
// can access (setup parameters are client-controlled and can be forged).
var ErrNotYours = errors.New("installation not accessible to this GitHub user")

// Installation is the part of a GitHub installation Shippin records.
type Installation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
	RepoSelection string  `json:"repository_selection"`
	SuspendedAt   *string `json:"suspended_at"`
}

// VerifyInstall swaps the one-time OAuth code for a user token, confirms installation inst is in
// that user's /user/installations, and drops the token. It returns the installation's details.
func (a *App) VerifyInstall(ctx context.Context, code string, inst int64) (*Installation, error) {
	tok, err := a.userToken(ctx, code)
	if err != nil {
		return nil, err
	}
	auth := "Bearer " + tok
	for page := 1; page <= 10; page++ {
		var out struct {
			Installations []Installation `json:"installations"`
		}
		u := fmt.Sprintf("%s/user/installations?per_page=100&page=%d", a.API, page)
		if err := a.call(ctx, "GET", u, auth, nil, &out); err != nil {
			return nil, err
		}
		for i := range out.Installations {
			if out.Installations[i].ID == inst {
				return &out.Installations[i], nil
			}
		}
		if len(out.Installations) < 100 {
			break
		}
	}
	return nil, ErrNotYours
}

func (a *App) userToken(ctx context.Context, code string) (string, error) {
	form := url.Values{"client_id": {a.ClientID}, "client_secret": {a.ClientSecret}, "code": {code}}
	req, err := http.NewRequestWithContext(ctx, "POST", a.Web+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := a.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if res.StatusCode >= 300 || json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&out) != nil || out.AccessToken == "" {
		return "", errors.New("github sign-in code was not accepted")
	}
	return out.AccessToken, nil
}

// AppInstallation reads one installation as the App (for polling and webhooks).
func (a *App) AppInstallation(ctx context.Context, inst int64) (*Installation, error) {
	auth, err := a.appAuth()
	if err != nil {
		return nil, err
	}
	out := &Installation{}
	return out, a.call(ctx, "GET", fmt.Sprintf("%s/app/installations/%d", a.API, inst), auth, nil, out)
}
