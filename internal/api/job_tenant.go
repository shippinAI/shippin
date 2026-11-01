package api

import (
	"context"
	"errors"

	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/projects"
)

// reconcilerActor is created_by / audit actor for work the console starts by itself.
const reconcilerActor = "system:reconciler"

// runTenantJob re-runs idempotent tenant provisioning (hostd profile, network, SSH address) for a
// tenant whose provisioning call died with the console.
func (a *API) runTenantJob(ctx context.Context, j *jobs.Job, step func(string)) (any, error) {
	t, err := a.db.TenantByID(ctx, j.TargetID)
	if err != nil {
		return nil, err
	}
	if t == nil || (t.Status != "provisioning" && t.Status != "failed") {
		return map[string]string{"skipped": "tenant is not provisioning"}, nil
	}
	step("Provisioning tenant network")
	nt, err := a.ensureTenant(ctx, t.IdpOrgID, t.Name, t.Plan)
	if errors.Is(err, errTenantLocked) || hostclient.Rejected(err) {
		return nil, jobs.Permanent(err)
	}
	if err != nil {
		return nil, err
	}
	return map[string]string{"status": nt.Status}, nil
}

// runBoundaryJob creates the tenant VM for a planned boundary, binds it, and places the tenant's
// pending projects on it. One VM per tenant: a second planned boundary fails permanently.
func (a *API) runBoundaryJob(ctx context.Context, j *jobs.Job, step func(string)) (any, error) {
	st := &projects.Store{Pool: a.db.Pool}
	state, _, err := st.BoundaryState(ctx, j.TenantID, j.TargetID)
	if errors.Is(err, projects.ErrNotFound) {
		return nil, jobs.Permanent(err)
	}
	if err != nil {
		return nil, err
	}
	if state == "ready" || state == "deleting" {
		return map[string]string{"skipped": "boundary is " + state}, nil
	}
	t, err := a.db.TenantByID(ctx, j.TenantID)
	if err != nil || t == nil {
		return nil, errors.Join(errors.New("tenant lookup failed"), err)
	}
	if t.Status != "active" {
		return nil, errors.New("tenant is " + t.Status + "; waiting for it to be active")
	}
	fail := func(err error) (any, error) {
		if hostclient.Rejected(err) || errors.Is(err, projects.ErrVMTaken) {
			err = jobs.Permanent(err)
		}
		if jobs.IsPermanent(err) || j.Attempts >= j.MaxAttempts {
			_ = st.SetBoundaryState(context.WithoutCancel(ctx), t.ID, j.TargetID, "error", err.Error())
		}
		return nil, err
	}
	step("Creating tenant VM")
	_ = st.SetBoundaryState(ctx, t.ID, j.TargetID, "provisioning", "")
	v, err := a.host.EnableTenantVM(ctx, t.ID, t.Plan)
	if err != nil {
		return fail(err)
	}
	if v.Mode != "vm" || v.Name == "" {
		return fail(jobs.Permanent(errors.New("hostd did not report a tenant VM")))
	}
	step("Recording boundary")
	if err := st.BindVM(ctx, t.ID, j.TargetID, v.Name, v.CPU, v.MemLimit, v.RootSize, v.DataSize); err != nil {
		return fail(err)
	}
	return map[string]string{"vm": v.Name}, nil
}
