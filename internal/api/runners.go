package api

import (
	"context"
	"errors"
	"log"
	"time"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/runtime/runner"
	"shippin.cloud/console/internal/runtime/runner_job"
	"shippin.cloud/console/internal/servers"
)

// runnerActor is who the infrastructure history names for machines build runners start and remove.
const runnerActor = "system:runners"

// runnersOn says whether this panel starts runner machines: the build runners section on, the job
// queue running, and a database and GitHub App to work with.
func (a *API) runnersOn(ctx context.Context) bool {
	if a.jobRunner() == nil || a.db == nil || !a.liveRunners(ctx).Enabled {
		return false
	}
	app, _ := a.github(ctx)
	return app != nil
}

func (a *API) runnerDeps(ctx context.Context) runner.Deps {
	return runner.Deps{Pool: a.db.Pool, GitHub: githubPort{a}, Machines: machinePort{a},
		JobLimit: time.Duration(a.liveRunners(ctx).JobLimitMinutes) * time.Minute, Limits: a.limits()}
}

// enqueueRunner adds a runner job to the durable queue, joining a live one for the same target.
func (a *API) enqueueRunner(ctx context.Context, kind, tenant, target string) error {
	targetType := "runner_job"
	if kind == jobs.KindRunnerRemove {
		targetType = "runner_machine"
	}
	_, created, err := a.jobs.Enqueue(ctx, jobs.Job{TenantID: tenant, Kind: kind, TargetType: targetType, TargetID: target,
		MaxAttempts: a.liveJobs(ctx).MaxAttempts, CreatedBy: runnerActor})
	if r := a.jobRunner(); err == nil && created && r != nil {
		r.Wake()
	}
	return err
}

// runRunnerProvision is the runner.provision job: start the machine for one runner job.
func (a *API) runRunnerProvision(ctx context.Context, j *jobs.Job, step func(string)) (any, error) {
	step("Starting a runner machine")
	return runner.Provision(ctx, a.runnerDeps(ctx), j.TargetID, j.Attempts >= j.MaxAttempts)
}

// runRunnerRemove is the runner.remove job: delete one machine, then start what was waiting for its room.
func (a *API) runRunnerRemove(ctx context.Context, j *jobs.Job, step func(string)) (any, error) {
	step("Removing a runner machine")
	tenant, err := runner.Remove(ctx, a.runnerDeps(ctx), j.TargetID)
	if err != nil {
		return nil, err
	}
	if tenant != "" {
		if err := runner.StartWaitingUnder(ctx, a.db.Pool, a.limits(), a.enqueueRunner, tenant); err != nil {
			log.Printf("runners: start waiting jobs of %s: %v", tenant, err)
		}
	}
	return map[string]string{"removed": j.TargetID}, nil
}

// sweepRunners is the reconciler's pass over runner machines.
func (a *API) sweepRunners(ctx context.Context) {
	if !a.runnersOn(ctx) {
		return
	}
	if err := runner.Sweep(ctx, a.runnerDeps(ctx), a.enqueueRunner); err != nil {
		log.Printf("reconcile: runners: %v", err)
	}
}

// runnerJobMoved follows a workflow_job delivery Receive recorded: a waiting job is queued to start,
// and a job that ended has its machine removed.
func (a *API) runnerJobMoved(ctx context.Context, w runner_job.Webhook, out runner_job.Outcome) {
	if out.JobID == "" || !a.runnersOn(ctx) {
		return
	}
	var tenant string
	if err := a.db.Pool.QueryRow(ctx, `SELECT tenant_id::text FROM shippin_runner_jobs WHERE id::text=$1`, out.JobID).Scan(&tenant); err != nil {
		log.Printf("runners: job %s: %v", out.JobID, err)
		return
	}
	if out.Status == runner_job.Queued {
		if err := a.enqueueRunner(ctx, jobs.KindRunnerProvision, tenant, out.JobID); err != nil {
			log.Printf("runners: queue a start for job %s: %v", out.JobID, err)
		}
		return
	}
	ids, err := runner.JobMoved(ctx, a.db.Pool, out.JobID, string(out.Status), w.RunnerName)
	if err != nil {
		log.Printf("runners: job %s moved on: %v", out.JobID, err)
	}
	for _, id := range ids {
		if err := a.enqueueRunner(ctx, jobs.KindRunnerRemove, tenant, id); err != nil {
			log.Printf("runners: queue removal of machine %s: %v", id, err)
		}
	}
}

// JITConfig registers a single-use runner through the panel's GitHub App.
func (p githubPort) JITConfig(ctx context.Context, installation, repository int64, fullName, runnerName string, labels []string) (string, int64, error) {
	app, _, err := p.a.githubOn(ctx)
	if err != nil {
		return "", 0, err
	}
	return app.GenerateJITConfig(ctx, installation, repository, fullName, runnerName, labels)
}

// machinePort is ports.Machines on the hosts of the fleet configured for runners.
type machinePort struct{ a *API }

func (p machinePort) Hosts(ctx context.Context) ([]string, error) {
	rc := p.a.liveRunners(ctx)
	if !rc.Enabled || p.a.db == nil {
		return nil, nil
	}
	rows, err := p.a.db.Pool.Query(ctx, `SELECT id::text FROM servers WHERE fleet_id::text=$1 AND hv_kind IS NOT NULL AND state <> 'revoked' ORDER BY slug`,
		rc.Fleet)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Create makes and starts the machine as a virtual machine, recorded in the host's change history.
func (p machinePort) Create(ctx context.Context, server string, m ports.Machine) (string, error) {
	a := p.a
	ep, err := a.hvDir().Endpoint(ctx, server)
	if err != nil {
		return "", err
	}
	gm, err := a.guestManager(ctx, server)
	if err != nil {
		return "", err
	}
	spec := hv.GuestSpec{Kind: hv.KindVM, Name: m.Name, Image: m.Image, Cores: m.Cores, Memory: m.MemoryMiB, Disk: m.DiskGiB,
		Storage: a.liveRunners(ctx).Storage, UserData: m.UserData, Start: true}
	if remote, alias, ok := a.uiConfig(ctx).ImageRemote(ep.Kind, m.Image); ok {
		spec.Image, spec.ImageServer, spec.ImageProtocol = alias, remote.Server, remote.Protocol
	}
	g, err := a.machineOperation(ctx, server, m.Name, "create", func(ctx context.Context) (hv.Guest, error) { return gm.CreateGuest(ctx, spec) })
	if err != nil {
		return "", err
	}
	if g.ID != "" {
		return g.ID, nil
	}
	return m.Name, nil
}

// Delete removes the machine; one the hypervisor says is gone already counts as removed.
func (p machinePort) Delete(ctx context.Context, server, guest string) error {
	gm, err := p.a.guestManager(ctx, server)
	if err != nil {
		return err
	}
	_, err = p.a.machineOperation(ctx, server, guest, "delete", func(ctx context.Context) (hv.Guest, error) {
		err := gm.DeleteGuest(ctx, guest)
		if errors.Is(err, hv.ErrGuestNotFound) {
			return hv.Guest{}, nil
		}
		return hv.Guest{}, err
	})
	return err
}

// machineOperation records a runner machine change in the host's history, as the panel's own work.
func (a *API) machineOperation(ctx context.Context, server, guest, verb string, fn func(context.Context) (hv.Guest, error)) (hv.Guest, error) {
	id, err := a.hvDir().StartOperation(ctx, servers.Operation{ServerID: server, Guest: guest, Verb: verb, Actor: runnerActor, ActorName: "Build runners"})
	if err != nil {
		return hv.Guest{}, errors.New("the change could not be recorded, so it was not made: " + err.Error())
	}
	g, err := fn(ctx)
	a.finishOperation(ctx, id, g, err)
	return g, err
}
