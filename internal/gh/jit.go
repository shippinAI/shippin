package gh

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// GenerateJITConfig registers a single-use ("just-in-time") self-hosted runner on one repository and
// returns its encoded configuration and the runner's id. The runner takes one job and is removed by
// GitHub once it exits.
//
// The token is narrowed to that one repository with administration: write, the permission GitHub
// requires for it. The encoded configuration is a credential: callers hand it to the runner's first
// boot only, and never log, store or return it.
func (a *App) GenerateJITConfig(ctx context.Context, inst int64, repoID int64, repoFullName, runnerName string, labels []string) (string, int64, error) {
	owner, repo, ok := strings.Cut(repoFullName, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", 0, fmt.Errorf("github: %q is not owner/repository", repoFullName)
	}
	if repoID <= 0 || runnerName == "" || len(labels) == 0 {
		return "", 0, errors.New("github: a just-in-time runner needs a repository id, a name and at least one label")
	}
	t, err := a.InstallationToken(ctx, inst, []int64{repoID}, map[string]string{"administration": "write"})
	if err != nil {
		return "", 0, err
	}
	in := map[string]any{"name": runnerName, "runner_group_id": 1, "labels": labels, "work_folder": "_work"}
	var out struct {
		Runner struct {
			ID int64 `json:"id"`
		} `json:"runner"`
		Config string `json:"encoded_jit_config"`
	}
	u := a.API + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/actions/runners/generate-jitconfig"
	if err := a.call(ctx, "POST", u, "token "+t.Value, in, &out); err != nil {
		return "", 0, err
	}
	if out.Config == "" || out.Runner.ID <= 0 {
		return "", 0, errors.New("github: the just-in-time runner answer has no configuration")
	}
	return out.Config, out.Runner.ID, nil
}
