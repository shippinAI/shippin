package gh

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Token is a 1-hour installation access token (ghs_…), narrowed to repos and permissions.
type Token struct {
	Value     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type cached struct {
	tok   Token
	until time.Time
}

// cacheFor is how long a token is reused: at most 50 min, and never past 10 min before expiry.
const cacheFor = 50 * time.Minute

// InstallationToken mints (or reuses) a token for installation inst limited to repoIDs and perms
// (e.g. {"contents":"write"}). An empty repoIDs means every repo the installation can see.
func (a *App) InstallationToken(ctx context.Context, inst int64, repoIDs []int64, perms map[string]string) (Token, error) {
	ids := slices.Clone(repoIDs)
	slices.Sort(ids)
	keys := make([]string, 0, len(perms))
	for k, v := range perms {
		keys = append(keys, k+"="+v)
	}
	slices.Sort(keys)
	ck := fmt.Sprintf("%d|%v|%s", inst, ids, strings.Join(keys, ","))
	now := a.now()
	a.mu.Lock()
	if c, ok := a.cache[ck]; ok && now.Before(c.until) {
		a.mu.Unlock()
		return c.tok, nil
	}
	a.mu.Unlock()
	auth, err := a.appAuth()
	if err != nil {
		return Token{}, err
	}
	body := map[string]any{}
	if len(ids) > 0 {
		body["repository_ids"] = ids
	}
	if len(perms) > 0 {
		body["permissions"] = perms
	}
	var t Token
	if err := a.call(ctx, "POST", fmt.Sprintf("%s/app/installations/%d/access_tokens", a.API, inst), auth, body, &t); err != nil {
		return Token{}, err
	}
	if !strings.HasPrefix(t.Value, "ghs_") {
		return Token{}, fmt.Errorf("unexpected installation token shape")
	}
	until := now.Add(cacheFor)
	if soon := t.ExpiresAt.Add(-10 * time.Minute); soon.Before(until) {
		until = soon
	}
	a.mu.Lock()
	a.cache[ck] = cached{t, until}
	a.mu.Unlock()
	return t, nil
}
