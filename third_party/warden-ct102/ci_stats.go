package main

// CI job stats — wide-event ingestion into Postgres (app_ci.job).
// Spec: docs/specs/runner-fleet-health-and-stats.md §3 (P2).
//
// Fast path: GitHub workflow_job webhooks (ci_stats_handlers.go).
// Truth-keeper: hourly reconciliation poller that sweeps recent runs per
// repo, backfills missed deliveries, and enriches event/pr_number which
// the job webhook payload does not carry.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type CIJob struct {
	ID          int64
	RunID       int64
	Repo        string
	Workflow    string
	JobName     string
	RunnerName  string
	HeadSHA     string
	HeadBranch  string
	Event       string
	PRNumber    int
	Conclusion  string
	QueuedAt    *time.Time
	StartedAt   time.Time
	CompletedAt time.Time
	HTMLURL     string
}

func ciJobUpsert(ctx context.Context, j CIJob) error {
	pool := getSupabasePool()
	if pool == nil {
		return fmt.Errorf("supabase pool unavailable")
	}
	_, err := pool.Exec(ctx, `
		insert into app_ci.job
		  (id, run_id, repo, workflow, job_name, runner_name, head_sha,
		   head_branch, event, pr_number, conclusion, queued_at, started_at,
		   completed_at, html_url)
		values ($1,$2,$3,$4,$5,nullif($6,''),$7,nullif($8,''),nullif($9,''),
		        nullif($10,0),$11,$12,$13,$14,nullif($15,''))
		on conflict (id) do update set
		  conclusion  = excluded.conclusion,
		  runner_name = coalesce(excluded.runner_name, app_ci.job.runner_name),
		  event       = coalesce(excluded.event, app_ci.job.event),
		  pr_number   = coalesce(excluded.pr_number, app_ci.job.pr_number)`,
		j.ID, j.RunID, j.Repo, j.Workflow, j.JobName, j.RunnerName, j.HeadSHA,
		j.HeadBranch, j.Event, j.PRNumber, j.Conclusion, j.QueuedAt,
		j.StartedAt, j.CompletedAt, j.HTMLURL)
	return err
}

// workflowJobPayload is the subset of the workflow_job webhook we keep.
type workflowJobPayload struct {
	Action      string `json:"action"`
	WorkflowJob struct {
		ID           int64      `json:"id"`
		RunID        int64      `json:"run_id"`
		Name         string     `json:"name"`
		WorkflowName string     `json:"workflow_name"`
		Conclusion   string     `json:"conclusion"`
		CreatedAt    *time.Time `json:"created_at"`
		StartedAt    time.Time  `json:"started_at"`
		CompletedAt  *time.Time `json:"completed_at"`
		RunnerName   string     `json:"runner_name"`
		HeadSHA      string     `json:"head_sha"`
		HeadBranch   string     `json:"head_branch"`
		HTMLURL      string     `json:"html_url"`
	} `json:"workflow_job"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func parseWorkflowJobEvent(body []byte) (CIJob, bool, error) {
	var p workflowJobPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return CIJob{}, false, err
	}
	if p.Action != "completed" || p.WorkflowJob.CompletedAt == nil {
		return CIJob{}, false, nil
	}
	return CIJob{
		ID:          p.WorkflowJob.ID,
		RunID:       p.WorkflowJob.RunID,
		Repo:        p.Repository.FullName,
		Workflow:    p.WorkflowJob.WorkflowName,
		JobName:     p.WorkflowJob.Name,
		RunnerName:  p.WorkflowJob.RunnerName,
		HeadSHA:     p.WorkflowJob.HeadSHA,
		HeadBranch:  p.WorkflowJob.HeadBranch,
		Conclusion:  p.WorkflowJob.Conclusion,
		QueuedAt:    p.WorkflowJob.CreatedAt,
		StartedAt:   p.WorkflowJob.StartedAt,
		CompletedAt: *p.WorkflowJob.CompletedAt,
		HTMLURL:     p.WorkflowJob.HTMLURL,
	}, true, nil
}

// --- reconciliation poller ------------------------------------------------

func ciStatsReconcileLoop() {
	if strings.TrimSpace(os.Getenv("WARDEN_GITHUB_TOKEN")) == "" {
		log.Printf("ci stats reconciler disabled: WARDEN_GITHUB_TOKEN not set")
		return
	}
	ciStatsReconcile()
	for range time.Tick(1 * time.Hour) {
		ciStatsReconcile()
	}
}

func ciStatsReconcile() {
	repoSet := map[string]bool{}
	for _, r := range loadCIRunners() {
		for _, repo := range r.Repos {
			repoSet[repo] = true
		}
	}
	total := 0
	for repo := range repoSet {
		n, err := reconcileRepoRuns(repo, 20)
		if err != nil {
			log.Printf("Ci stats reconcile failed for %q: %v", repo, err)
			continue
		}
		total += n
	}
	log.Printf("ci stats reconcile pass complete, %d jobs upserted", total)
}

func reconcileRepoRuns(repo string, maxRuns int) (int, error) {
	var runs struct {
		WorkflowRuns []struct {
			ID           int64  `json:"id"`
			Event        string `json:"event"`
			Status       string `json:"status"`
			PullRequests []struct {
				Number int `json:"number"`
			} `json:"pull_requests"`
		} `json:"workflow_runs"`
	}
	if err := githubGetJSON(fmt.Sprintf("/repos/%s/actions/runs?per_page=%d", repo, maxRuns), &runs); err != nil {
		return 0, err
	}
	count := 0
	for _, run := range runs.WorkflowRuns {
		if run.Status != "completed" {
			continue
		}
		var jobs struct {
			Jobs []json.RawMessage `json:"jobs"`
		}
		if err := githubGetJSON(fmt.Sprintf("/repos/%s/actions/runs/%d/jobs?per_page=100", repo, run.ID), &jobs); err != nil {
			continue
		}
		for _, raw := range jobs.Jobs {
			wrapped := []byte(fmt.Sprintf(`{"action":"completed","workflow_job":%s,"repository":{"full_name":%q}}`, raw, repo))
			job, ok, err := parseWorkflowJobEvent(wrapped)
			if err != nil || !ok {
				continue
			}
			job.Event = run.Event
			if len(run.PullRequests) > 0 {
				job.PRNumber = run.PullRequests[0].Number
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err = ciJobUpsert(ctx, job)
			cancel()
			if err == nil {
				count++
			}
		}
	}
	return count, nil
}

func githubGetJSON(path string, out any) error {
	token := strings.TrimSpace(os.Getenv("WARDEN_GITHUB_TOKEN"))
	req, err := http.NewRequest("GET", "https://api.github.com"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
