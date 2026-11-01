package api

import (
	"errors"
	"log"
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/jobs"
)

// jobOut is the OpenAPI Job shape.
type jobOut struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	Kind       string     `json:"kind"`
	Target     jobTarget  `json:"target"`
	Status     string     `json:"status"`
	Message    string     `json:"message,omitempty"`
	Attempts   int        `json:"attempts"`
	Error      *jobError  `json:"error,omitempty"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type jobTarget struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type jobError struct {
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

// jobView renders a job; raw errors are shown to operators only.
func jobView(j *jobs.Job, operator bool) *jobOut {
	if j == nil {
		return nil
	}
	o := &jobOut{ID: j.ID, TenantID: j.TenantID, Kind: j.Kind, Target: jobTarget{j.TargetType, j.TargetID},
		Status: j.Status, Message: j.Step, Attempts: j.Attempts, CreatedBy: j.CreatedBy,
		CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt}
	if j.Error != "" && (j.Status == jobs.Failed || operator) {
		o.Error = &jobError{Title: "The operation failed."}
		if operator {
			o.Error.Detail = j.Error
		}
	}
	return o
}

// getJob: GET /v1/jobs/{jobId} for members of the job's tenant and operators (others: 404).
func (a *API) getJob(w http.ResponseWriter, r *http.Request) {
	c, ok := a.resolve(w, r)
	if !ok {
		return
	}
	j, err := a.jobs.Get(r.Context(), r.PathValue("jobId"))
	if errors.Is(err, jobs.ErrNotFound) || (err == nil && !c.seesJobs(j.TenantID)) {
		auth.Problem(w, http.StatusNotFound, "Job not found.")
		return
	}
	if err != nil {
		log.Printf("job %s: %v", r.PathValue("jobId"), err)
		auth.Problem(w, http.StatusServiceUnavailable, "Jobs are unavailable right now.")
		return
	}
	writeJSON(w, jobView(j, c.op != ""))
}

// seesJobs: operators, and members of the tenant (a grant to one workspace isn't membership).
func (c *caller) seesJobs(tenant string) bool {
	if c.op != "" {
		return true
	}
	for _, t := range c.tenants {
		if t.ID == tenant && len(t.grants) == 0 && len(t.Roles) > 0 {
			return true
		}
	}
	return false
}
