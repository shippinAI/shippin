package main

// Read-side queries + fun-layer computation over app_ci.job.
// Spec: docs/specs/runner-fleet-health-and-stats.md §5 (P3).
// Fun stats are computed in Go from plain selects — zero extra storage,
// delete-safe, unit-testable. SQL views can graduate with rollups later.

import (
	"context"
	"fmt"
	"time"
)

type CIJobRow struct {
	Repo        string    `json:"repo"`
	Workflow    string    `json:"workflow"`
	JobName     string    `json:"job_name"`
	RunnerName  string    `json:"runner_name"`
	Conclusion  string    `json:"conclusion"`
	ExecMS      int       `json:"exec_ms"`
	CompletedAt time.Time `json:"completed_at"`
	HTMLURL     string    `json:"html_url"`
}

type CIStreak struct {
	Repo    string `json:"repo"`
	Current int    `json:"current"`
	Best    int    `json:"best"`
}

type CIRaceRow struct {
	HeadSHA     string    `json:"head_sha"`
	WindowsMS   int       `json:"windows_ms"`
	LinuxMS     int       `json:"linux_ms"`
	WindowsMin  float64   `json:"windows_min"`
	LinuxMin    float64   `json:"linux_min"`
	CompletedAt time.Time `json:"completed_at"`
}

type CIAchievement struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Achieved bool   `json:"achieved"`
	Detail   string `json:"detail"`
}

type CIStats struct {
	Available    bool            `json:"available"`
	Error        string          `json:"error,omitempty"`
	RecentJobs   []CIJobRow      `json:"recent_jobs"`
	Streaks      []CIStreak      `json:"streaks"`
	Race         []CIRaceRow     `json:"race"`
	Achievements []CIAchievement `json:"achievements"`
	TotalGreen   int             `json:"total_green"`
}

// computeStreaks takes conclusions ordered NEWEST FIRST and returns the
// current leading streak of successes and the best streak anywhere.
func computeStreaks(conclusions []string) (current, best int) {
	run := 0
	leading := true
	for _, c := range conclusions {
		if c == "success" {
			run++
			if run > best {
				best = run
			}
			if leading {
				current++
			}
		} else {
			run = 0
			leading = false
		}
	}
	return current, best
}

func collectCIStats(ctx context.Context) CIStats {
	stats := CIStats{}
	pool := getSupabasePool()
	if pool == nil {
		stats.Error = "stats store unavailable (Supabase pool not initialized)"
		return stats
	}
	stats.Available = true

	rows, err := pool.Query(ctx, `
		select repo, workflow, job_name, coalesce(runner_name,''), conclusion,
		       coalesce(exec_ms,0), completed_at, coalesce(html_url,'')
		from app_ci.job order by completed_at desc limit 10`)
	if err != nil {
		stats.Available, stats.Error = false, err.Error()
		return stats
	}
	for rows.Next() {
		var j CIJobRow
		if err := rows.Scan(&j.Repo, &j.Workflow, &j.JobName, &j.RunnerName,
			&j.Conclusion, &j.ExecMS, &j.CompletedAt, &j.HTMLURL); err == nil {
			stats.RecentJobs = append(stats.RecentJobs, j)
		}
	}
	rows.Close()

	// Nightly streaks per repo (newest first, bounded window).
	srows, err := pool.Query(ctx, `
		select repo, conclusion from app_ci.job
		where workflow ilike '%nightly%'
		order by completed_at desc limit 800`)
	if err == nil {
		perRepo := map[string][]string{}
		order := []string{}
		for srows.Next() {
			var repo, conclusion string
			if err := srows.Scan(&repo, &conclusion); err == nil {
				if _, seen := perRepo[repo]; !seen {
					order = append(order, repo)
				}
				perRepo[repo] = append(perRepo[repo], conclusion)
			}
		}
		srows.Close()
		for _, repo := range order {
			cur, best := computeStreaks(perRepo[repo])
			stats.Streaks = append(stats.Streaks, CIStreak{Repo: repo, Current: cur, Best: best})
		}
	}

	// The Race — per-commit Windows vs Linux exec time (successes only).
	rrows, err := pool.Query(ctx, `
		select head_sha, min(completed_at),
		       max(exec_ms) filter (where runner_name ilike '%win%'),
		       max(exec_ms) filter (where runner_name ilike '%lnx%' or runner_name ilike '%linux%')
		from app_ci.job
		where conclusion = 'success'
		group by head_sha
		having max(exec_ms) filter (where runner_name ilike '%win%') is not null
		   and max(exec_ms) filter (where runner_name ilike '%lnx%' or runner_name ilike '%linux%') is not null
		order by min(completed_at) desc limit 14`)
	if err == nil {
		for rrows.Next() {
			var r CIRaceRow
			if err := rrows.Scan(&r.HeadSHA, &r.CompletedAt, &r.WindowsMS, &r.LinuxMS); err == nil {
				r.WindowsMin = float64(r.WindowsMS) / 60000
				r.LinuxMin = float64(r.LinuxMS) / 60000
				stats.Race = append(stats.Race, r)
			}
		}
		rrows.Close()
	}

	var greens, absorbMax, subFiveMS int
	_ = pool.QueryRow(ctx, `select count(*) from app_ci.job where conclusion='success'`).Scan(&greens)
	_ = pool.QueryRow(ctx, `
		select coalesce(max(n),0) from (
		  select count(distinct pr_number) as n from app_ci.job
		  where pr_number is not null and conclusion='success'
		  group by repo, date(completed_at)) d`).Scan(&absorbMax)
	_ = pool.QueryRow(ctx, `
		select coalesce(min(exec_ms),0) from app_ci.job
		where conclusion='success' and runner_name ilike '%win%'
		  and workflow ilike '%nightly%'`).Scan(&subFiveMS)
	stats.TotalGreen = greens

	bestNightly := 0
	for _, s := range stats.Streaks {
		if s.Current > bestNightly {
			bestNightly = s.Current
		}
	}
	stats.Achievements = []CIAchievement{
		{ID: "century", Label: "Century", Achieved: greens >= 100,
			Detail: fmt.Sprintf("%d green builds of 100", greens)},
		{ID: "night-owl", Label: "Night Owl", Achieved: bestNightly >= 30,
			Detail: fmt.Sprintf("nightly streak %d of 30", bestNightly)},
		{ID: "sub-five", Label: "Sub-Five", Achieved: subFiveMS > 0 && subFiveMS < 5*60*1000,
			Detail: fmt.Sprintf("fastest Windows nightly %.1f min", float64(subFiveMS)/60000)},
		{ID: "absorption", Label: "Absorption Event", Achieved: absorbMax >= 5,
			Detail: fmt.Sprintf("best PR day: %d merged green of 5", absorbMax)},
	}
	return stats
}
