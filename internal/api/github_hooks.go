package api

import (
	"context"
	"io"
	"log"
	"net/http"
	"regexp"

	"shippin.cloud/console/internal/integrations/gh"
	"shippin.cloud/console/internal/runtime/runner_job"
)

// webhookMax caps a webhook body; GitHub's are far smaller for the events subscribed.
const webhookMax = 1 << 20

var deliveryRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// githubWebhook: no session; the HMAC signature is the only authentication. Replays (same
// X-GitHub-Delivery) are acknowledged and ignored.
func (a *API) githubWebhook(w http.ResponseWriter, r *http.Request) {
	app, g := a.github(r.Context())
	if app == nil {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookMax))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !gh.VerifySignature(g.WebhookSecret, body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}
	id := r.Header.Get("X-GitHub-Delivery")
	if !deliveryRe.MatchString(id) {
		http.Error(w, "bad delivery id", http.StatusBadRequest)
		return
	}
	fresh, err := a.db.RecordDelivery(r.Context(), id)
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if fresh {
		if e, err := gh.ParseEvent(body); err == nil {
			a.applyEvent(r.Context(), r.Header.Get("X-GitHub-Event"), e)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// applyEvent maps installation, installation_repositories and repository events onto sources, and
// workflow_job events onto the organization's runner jobs (internal/runtime/runner_job) and their
// machines (internal/runtime/runner).
func (a *API) applyEvent(ctx context.Context, kind string, e *gh.Event) {
	inst := e.Installation.ID
	var err error
	switch {
	case kind == "installation" && e.Action == "deleted":
		err = a.db.SetInstallationState(ctx, inst, "deleted")
	case kind == "installation" && e.Action == "suspend":
		err = a.db.SetInstallationState(ctx, inst, "suspended")
	case kind == "installation" && e.Action == "unsuspend":
		err = a.db.SetInstallationState(ctx, inst, "active")
	case kind == "installation_repositories" && e.Action == "removed":
		ids := make([]int64, 0, len(e.Removed))
		for _, r := range e.Removed {
			ids = append(ids, r.ID)
		}
		err = a.db.RevokeRepos(ctx, inst, ids)
	case kind == "repository" && e.Action == "renamed" && e.Repository != nil:
		err = a.db.RenameRepo(ctx, e.Repository.ID, e.Repository.FullName)
	case kind == "workflow_job" && e.WorkflowJob != nil && e.Repository != nil:
		j := e.WorkflowJob
		var out runner_job.Outcome
		hook := runner_job.Webhook{Action: e.Action, InstallationID: inst,
			RepositoryID: e.Repository.ID, Repository: e.Repository.FullName, JobID: j.ID, RunID: j.RunID, Name: j.Name,
			Labels: j.Labels, RunnerName: j.RunnerName, Conclusion: j.Conclusion, StartedAt: j.StartedAt, CompletedAt: j.CompletedAt}
		out, err = runner_job.ReceiveUnder(ctx, a.db.Pool, a.limits(), hook)
		if err == nil && out.Status == runner_job.Refused {
			log.Printf("github webhook: runner job %s refused: %s", out.JobID, out.Reason)
		}
		if err == nil {
			// Start a waiting job's machine, or remove the machine of one that ended (internal/runtime/runner).
			a.runnerJobMoved(ctx, hook, out)
		}
	}
	if err != nil {
		log.Printf("github webhook %s/%s: %v", kind, e.Action, err)
	}
}
