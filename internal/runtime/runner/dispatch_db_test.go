package runner_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/runtime/runner"
)

// Waiting jobs start in arrival order, only while the organization is under its plan's limit, one
// organization at a time in turn, and never twice.
func TestDispatchOrderLimitsAndFairness(t *testing.T) {
	w := newWorld(t, nil)
	ctx := t.Context()
	a1, a2, a3, a4 := w.queue(acme, 1), w.queue(acme, 2), w.queue(acme, 3), w.queue(acme, 4)
	g11, g12 := w.queue(globex, 11), w.queue(globex, 12)
	i21 := w.queue(initech, 21)

	// The sweep queues what each organization has room for, one from each in turn: Globex's first job
	// goes before Acme's second although it came later, and Initech's plan runs nothing.
	q := &enqueued{}
	if err := runner.Sweep(ctx, w.d, q.add); err != nil {
		t.Fatal(err)
	}
	want := []string{jobs.KindRunnerProvision + " " + acme + " " + a1, jobs.KindRunnerProvision + " " + globex + " " + g11, jobs.KindRunnerProvision + " " + acme + " " + a2}
	if strings.Join(q.jobs, "\n") != strings.Join(want, "\n") {
		t.Fatalf("queued starts:\n%s\nwant:\n%s", strings.Join(q.jobs, "\n"), strings.Join(want, "\n"))
	}

	// A later job does not pass an earlier one of its organization.
	if out := w.provision(a3, false); !strings.Contains(out.Waiting, "earlier jobs") || w.machines() != 0 {
		t.Fatalf("a3 went before a1 and a2: %+v", out)
	}
	if out := w.provision(a1, false); out.Started == "" {
		t.Fatalf("a1: %+v", out)
	}
	if out := w.provision(a2, false); out.Started == "" {
		t.Fatalf("a2: %+v", out)
	}
	// Acme is at its limit of two.
	if out := w.provision(a3, false); !strings.Contains(out.Waiting, "2 of its 2") {
		t.Fatalf("a3 over the limit: %+v", out)
	}
	if out := w.provision(i21, false); !strings.Contains(out.Waiting, "no runner machines") {
		t.Fatalf("Initech's plan allows none: %+v", out)
	}
	m1, m2 := w.machineFor(a1), w.machineFor(a2)
	if m1.state != "running" || m2.state != "running" || m1.server != host1 || m2.server != host2 {
		t.Fatalf("each on the least loaded host: %+v %+v", m1, m2)
	}

	// Running it again starts nothing: the job has its machine.
	if out := w.provision(a1, false); out.Started != "" || !strings.Contains(out.Skipped, "already running") {
		t.Fatalf("a1 again: %+v", out)
	}
	// Two runs at once for the same job make one machine: the second finds the first at work.
	w.m.hold, w.m.entered, w.m.release = true, make(chan struct{}), make(chan struct{})
	first := make(chan runner.Outcome)
	go func() {
		out, err := runner.Provision(ctx, w.d, g11, false)
		if err != nil {
			t.Error(err)
		}
		first <- out
	}()
	<-w.m.entered
	if out := w.provision(g11, false); !strings.Contains(out.Skipped, "another attempt") {
		t.Fatalf("the second run: %+v", out)
	}
	close(w.m.release)
	if out := <-first; out.Started == "" {
		t.Fatalf("the first run: %+v", out)
	}
	w.m.hold = false
	if w.m.creates != 3 || w.machines() != 3 {
		t.Fatalf("machines made: %d, rows %d, want 3", w.m.creates, w.machines())
	}
	if out := w.provision(g12, false); !strings.Contains(out.Waiting, "1 of its 1") {
		t.Fatalf("Globex at its limit: %+v", out)
	}
	_ = a4

	// GitHub was asked for a runner on the job's own repository, under the machine's name, with the
	// job's labels less GitHub's own; the machine got that configuration in its first-boot data only.
	c := w.gh.calls[0]
	if c.inst != 11 || c.repo != 101 || c.full != "acme/app" || c.name != m1.guest || strings.Join(c.labels, ",") != "shippin-2x" {
		t.Fatalf("GitHub was asked for %+v", c)
	}
	made := w.m.made[m1.guest]
	if made.Image != "runner-ubuntu" || made.Cores != 2 || made.MemoryMiB != 8192 || made.DiskGiB != 40 ||
		!strings.Contains(made.UserData, `"`+c.config+`"`) || !strings.Contains(made.UserData, "./run.sh --jitconfig") || !strings.Contains(made.UserData, "poweroff") {
		t.Fatalf("made %+v", made)
	}
	for _, c := range w.gh.calls {
		var n int
		if err := w.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM shippin_runner_machines m WHERE m::text LIKE '%'||$1||'%')
		  + (SELECT count(*) FROM shippin_runner_jobs j WHERE j::text LIKE '%'||$1||'%')`, c.config).Scan(&n); err != nil || n != 0 {
			t.Fatalf("the runner configuration was stored: %d %v", n, err)
		}
	}
	if raw, _ := json.Marshal(w.provision(a1, false)); strings.Contains(string(raw), "SeCrEt") {
		t.Fatalf("an outcome carries the configuration: %s", raw)
	}
}

// A job that has ended takes its machine with it; a machine that outlives the job time limit is
// removed whatever its job says; a failed removal is recorded and tried again; and one
// organization's deliveries never touch another's machines.
func TestRemoval(t *testing.T) {
	w := newWorld(t, nil)
	ctx := t.Context()
	a1, a2, a3 := w.queue(acme, 1), w.queue(acme, 2), w.queue(acme, 3)
	g11 := w.queue(globex, 11)
	for _, j := range []string{a1, a2, g11} {
		if out := w.provision(j, false); out.Started == "" {
			t.Fatalf("%s: %+v", j, out)
		}
	}
	m1, m2, mg := w.machineFor(a1), w.machineFor(a2), w.machineFor(g11)

	// Globex cannot end Acme's job, nor reach Acme's machine by naming its runner.
	if out, ids := w.receive(globex, "completed", 1, m1.guest, "success"); out.JobID != "" || len(ids) != 0 {
		t.Fatalf("Globex's delivery reached Acme's job: %+v %v", out, ids)
	}
	ids, err := runner.JobMoved(ctx, w.pool, g11, "completed", m1.guest)
	if err != nil || len(ids) != 1 || ids[0] != mg.id {
		t.Fatalf("a Globex job ending removes only Globex's machine: %v %v", ids, err)
	}
	if w.machineFor(a1).state != "running" {
		t.Fatal("Acme's machine was touched")
	}

	// a2 is taken by a1's runner and a1 by a2's: each machine goes with the job it ran.
	w.receive(acme, "in_progress", 2, m1.guest, "")
	w.receive(acme, "in_progress", 1, m2.guest, "")
	if _, ids := w.receive(acme, "completed", 2, m1.guest, "success"); len(ids) != 1 || ids[0] != m1.id {
		t.Fatalf("a2 ended on m1: %v", ids)
	}
	if w.machineFor(a2).state != "running" {
		t.Fatal("m2 was removed while its runner was still on a1")
	}
	if _, err := runner.Remove(ctx, w.d, m1.id); err != nil {
		t.Fatal(err)
	}
	if got := w.machineFor(a1); got.state != "removed" || got.reason != "Its job ended." || !contains(w.m.deleted, m1.guest) {
		t.Fatalf("m1 after its job ended: %+v deleted %v", got, w.m.deleted)
	}
	// Its room goes to the next waiting job.
	q := &enqueued{}
	if err := runner.StartWaiting(ctx, w.pool, q.add, acme); err != nil || len(q.jobs) != 1 || !strings.HasSuffix(q.jobs[0], a3) {
		t.Fatalf("after the removal: %v %v", q.jobs, err)
	}
	// A job GitHub cancels before any runner took it removes the machine started for it.
	if out := w.provision(a3, false); out.Started == "" {
		t.Fatalf("a3: %+v", out)
	}
	m3 := w.machineFor(a3)
	if out, ids := w.receive(acme, "completed", 3, "", "cancelled"); out.Status != "canceled" || len(ids) != 1 || ids[0] != m3.id {
		t.Fatalf("canceled a3: %+v %v", out, ids)
	}

	// m2 outlives the job time limit: the sweep marks it and queues its removal, with m3's.
	if _, err := w.pool.Exec(ctx, `UPDATE shippin_runner_machines SET created_at = now() - interval '2 hours' WHERE id::text=$1`, m2.id); err != nil {
		t.Fatal(err)
	}
	q = &enqueued{}
	if err := runner.Sweep(ctx, w.d, q.add); err != nil {
		t.Fatal(err)
	}
	if got := w.machineFor(a2); got.state != "removing" || !strings.Contains(got.reason, "longer than the job time limit") {
		t.Fatalf("m2 past the limit: %+v", got)
	}
	if !contains(q.jobs, jobs.KindRunnerRemove+" "+acme+" "+m2.id) || !contains(q.jobs, jobs.KindRunnerRemove+" "+acme+" "+m3.id) {
		t.Fatalf("removals queued: %v", q.jobs)
	}

	// The host refuses: m2 is left failed with the error, and the next sweep tries again.
	w.m.failDelete = errors.New("incus: delete HTTP 500: host is busy")
	if _, err := runner.Remove(ctx, w.d, m2.id); err == nil {
		t.Fatal("a failed delete was not reported")
	}
	if got := w.machineFor(a2); got.state != "failed" || !strings.Contains(got.err, "host is busy") {
		t.Fatalf("after a failed delete: %+v", got)
	}
	q = &enqueued{}
	_ = runner.Sweep(ctx, w.d, q.add)
	if !contains(q.jobs, jobs.KindRunnerRemove+" "+acme+" "+m2.id) {
		t.Fatalf("the failed removal is retried: %v", q.jobs)
	}
	w.m.failDelete = nil
	if _, err := runner.Remove(ctx, w.d, m2.id); err != nil {
		t.Fatal(err)
	}
	if got := w.machineFor(a2); got.state != "removed" || w.m.on[m2.guest] != "" {
		t.Fatalf("after the retry: %+v", got)
	}
	// Removing again does nothing.
	n := len(w.m.deleted)
	if _, err := runner.Remove(ctx, w.d, m2.id); err != nil || len(w.m.deleted) != n {
		t.Fatalf("a second removal deleted again: %v", err)
	}
}

// An attempt that fails is carried on by the next run with a fresh runner name, after removing what
// the failed attempt left; the last attempt failing leaves the machine failed, for removal, and the
// error never carries the runner configuration.
func TestStartFailure(t *testing.T) {
	w := newWorld(t, nil)
	ctx := t.Context()
	a1, a2 := w.queue(acme, 1), w.queue(acme, 2)

	w.m.failCreate = errors.New("incus: start failed")
	if _, err := runner.Provision(ctx, w.d, a1, false); err == nil || strings.Contains(err.Error(), "SeCrEt") {
		t.Fatalf("a failed attempt: %v", err)
	}
	first := w.machineFor(a1)
	if first.state != "starting" || !strings.Contains(first.err, "incus: start failed") || strings.Contains(first.err, "SeCrEt") ||
		!strings.Contains(first.err, "[runner configuration]") {
		t.Fatalf("after one failed attempt: %+v", first)
	}
	w.m.failCreate = nil
	if out := w.provision(a1, false); out.Started == "" {
		t.Fatalf("the second attempt: %+v", out)
	}
	second := w.machineFor(a1)
	if second.state != "running" || second.id != first.id || second.guest == first.guest || !strings.HasSuffix(second.guest, "-2") ||
		!contains(w.m.deleted, first.guest) || w.gh.calls[1].name != second.guest {
		t.Fatalf("carried on: %+v after %+v; deleted %v", second, first, w.m.deleted)
	}

	w.m.failCreate = errors.New("incus: no space left")
	if _, err := runner.Provision(ctx, w.d, a2, true); err == nil {
		t.Fatal("the last attempt failing was not reported")
	}
	failed := w.machineFor(a2)
	var reason string
	_ = w.pool.QueryRow(ctx, `SELECT reason FROM shippin_runner_jobs WHERE id::text=$1`, a2).Scan(&reason)
	if failed.state != "failed" || !strings.Contains(failed.err, "no space left") || !strings.Contains(reason, "could not start a runner") {
		t.Fatalf("after the last attempt: %+v, job says %q", failed, reason)
	}
	if out := w.provision(a2, false); !strings.Contains(out.Skipped, "already failed") {
		t.Fatalf("a failed job started again: %+v", out)
	}
	q := &enqueued{}
	_ = runner.Sweep(ctx, w.d, q.add)
	if !contains(q.jobs, jobs.KindRunnerRemove+" "+acme+" "+failed.id) {
		t.Fatalf("the failed machine is removed: %v", q.jobs)
	}
	if _, err := runner.Remove(ctx, w.d, failed.id); err != nil || w.machineFor(a2).state != "removed" {
		t.Fatalf("removing the failed machine: %v", err)
	}

	// GitHub refusing is recorded the same way, before anything is made.
	a3 := w.queue(acme, 3)
	w.gh.fail = errors.New("github answered 403")
	creates := w.m.creates
	if _, err := runner.Provision(ctx, w.d, a3, false); err == nil || w.m.creates != creates || !strings.Contains(w.machineFor(a3).err, "github answered 403") {
		t.Fatalf("GitHub refusing: %v, %+v", err, w.machineFor(a3))
	}
}

// User data is refused for a configuration that does not look like GitHub's.
func TestUserDataRefusesAnUnexpectedConfiguration(t *testing.T) {
	for _, bad := range []string{"", "a b", "x\n  - evil", `"quoted"`} {
		if _, err := runner.UserData(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	ud, err := runner.UserData("eyJhIjoiYiJ9")
	if err != nil || !strings.HasPrefix(ud, "#cloud-config\n") || !strings.Contains(ud, `content: "eyJhIjoiYiJ9"`) {
		t.Fatalf("%v\n%s", err, ud)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
