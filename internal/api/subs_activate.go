package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/subs"
)

type activateOut struct {
	Home         *subs.Home `json:"home"`
	Provider     string     `json:"provider"`
	TerminalURL  string     `json:"terminal_url"`
	LoginCommand string     `json:"login_command"`
	Job          *jobOut    `json:"job,omitempty"`
}

// aiJob is the ai.activate payload: start the terminal, then optionally sign out of one provider.
type aiJob struct {
	Logout string `json:"logout,omitempty"`
}

// activateSubscription starts the caller's own activation terminal (their AI home mounted) and
// tells them which vendor command to run in it. The sign-in happens between the user and the
// vendor; Shippin only ever learns whether it succeeded.
func (a *API) activateSubscription(w http.ResponseWriter, r *http.Request) {
	c, t, acct, ok := a.subsCtx(w, r, true)
	if !ok {
		return
	}
	p, _ := a.cfg.AI.Provider(r.PathValue("provider"))
	h, err := a.subs.EnsureHome(r.Context(), t.ID, acct, "", a.terminalHost(t.ID, acct))
	if err == nil && h.UtilityHost != a.terminalHost(t.ID, acct) {
		auth.Problem(w, http.StatusConflict, "Your AI runner needs the isolation upgrade before sign-in can start.")
		return
	}
	if err == nil {
		err = a.subs.SetStatus(r.Context(), h, p.ID, subs.Activating, "", false)
	}
	if err == nil {
		err = a.subs.SetUtility(r.Context(), h.ID, "starting", true)
	}
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Subscriptions are unavailable right now.")
		return
	}
	j, err := a.startAIJob(r.Context(), w, c, h, aiJob{})
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Your AI terminal could not be queued. Try again.")
		return
	}
	writeStatus(w, http.StatusAccepted, activateOut{Home: h, Provider: p.ID, TerminalURL: "https://" + h.UtilityHost + "/",
		LoginCommand: p.LoginCommand, Job: j})
}

// startAIJob enqueues ai.activate for home h, or runs it in the background with jobs disabled.
func (a *API) startAIJob(ctx context.Context, w http.ResponseWriter, c *caller, h *subs.Home, in aiJob) (*jobOut, error) {
	if a.runner == nil {
		go func() {
			if _, err := a.activateHome(context.Background(), h.ID, in); err != nil {
				log.Printf("ai home %s: %v", h.ID, err)
			}
		}()
		return nil, nil
	}
	payload, _ := json.Marshal(in)
	target := "ai_home" // a sign-out is its own job so it never folds into a live activation
	if in.Logout != "" {
		target = "ai_logout:" + in.Logout
	}
	j, _, err := a.jobs.Enqueue(ctx, jobs.Job{TenantID: h.TenantID, Kind: jobs.KindAIActivate, TargetType: target,
		TargetID: h.ID, MaxAttempts: a.cfg.Jobs.MaxAttempts, CreatedBy: c.id.Sub, Payload: payload})
	if err != nil {
		log.Printf("ai home %s: enqueue: %v", h.ID, err)
		_ = a.subs.SetUtility(ctx, h.ID, "error", false)
		return nil, err
	}
	a.runner.Wake()
	w.Header().Set("Location", "/v1/jobs/"+j.ID)
	return jobView(j, c.op != ""), nil
}

// runAIJob is the ai.activate handler.
func (a *API) runAIJob(ctx context.Context, j *jobs.Job, step func(string)) (any, error) {
	var in aiJob
	if len(j.Payload) > 0 && json.Unmarshal(j.Payload, &in) != nil {
		return nil, jobs.Permanent(errors.New("bad ai.activate payload"))
	}
	step("Starting your AI terminal")
	return a.activateHome(ctx, j.TargetID, in)
}

// activateHome starts the account's terminal container (creating the AI home volume on first use),
// then signs out of in.Logout if asked, and records the outcome.
func (a *API) activateHome(ctx context.Context, homeID string, in aiJob) (any, error) {
	h, err := a.subs.HomeByID(ctx, homeID)
	if err != nil {
		return nil, err
	}
	tn, err := a.db.TenantByID(ctx, h.TenantID)
	if err != nil || tn == nil {
		return nil, errors.Join(errors.New("tenant lookup failed"), err)
	}
	if _, err := a.host.StartUtility(ctx, h.TenantID, h.AccountID, tn.Plan, h.UtilityHost); err != nil {
		return nil, errors.Join(err, a.utilityFailed(ctx, h.ID))
	}
	if h.State != "ready" {
		_ = a.subs.SetHomeState(ctx, h.ID, "ready", "")
	}
	_ = a.subs.SetUtility(ctx, h.ID, "running", true)
	if in.Logout != "" {
		if err := a.host.UtilityLogout(ctx, h.TenantID, h.AccountID, in.Logout); err != nil {
			return nil, err
		}
		_ = a.subs.SetStatus(ctx, h, in.Logout, subs.Disconnected, "", true)
	}
	return map[string]any{"home": h.ID, "started_at": time.Now().UTC()}, nil
}
