import assert from "node:assert/strict";
import type { RunnerJob } from "../../lib/api/actions.gen";
import {
  ago,
  assignmentBody,
  assignmentFor,
  assignmentProblem,
  formatDuration,
  hasActiveJobs,
  jobCounts,
  jobDuration,
  jobStatusView,
  problemText,
  refusalText,
  repositoryState,
  runsOn,
} from "./model";

const job = (over: Partial<RunnerJob>): RunnerJob => ({
  id: "j1",
  github_job_id: "1",
  run_id: "2",
  repository_id: "3",
  repository: "acme/web",
  workspace_id: "w1",
  project_id: null,
  name: "build",
  labels: ["shippin-2x"],
  size: "2x",
  status: "queued",
  reason: "",
  conclusion: "",
  runner_name: "",
  queued_at: "2026-11-15T10:00:00Z",
  started_at: null,
  completed_at: null,
  seconds: null,
  ...over,
});

// Every status reads in words, with a tone and an icon beside them.
assert.deepEqual(jobStatusView(job({ status: "queued" })), {
  label: "Waiting",
  tone: "info",
  icon: "clock",
  hint: "Recorded and waiting for a runner.",
});
assert.equal(jobStatusView(job({ status: "refused" })).label, "Refused");
assert.equal(jobStatusView(job({ status: "refused" })).tone, "danger");
assert.equal(jobStatusView(job({ status: "in_progress" })).label, "Running");
assert.equal(jobStatusView(job({ status: "canceled" })).tone, "neutral");
// A completed job reads as GitHub's conclusion.
assert.equal(jobStatusView(job({ status: "completed" })).label, "Completed");
assert.equal(jobStatusView(job({ status: "completed", conclusion: "success" })).label, "Succeeded");
assert.equal(jobStatusView(job({ status: "completed", conclusion: "failure" })).tone, "danger");
assert.equal(jobStatusView(job({ status: "completed", conclusion: "timed_out" })).label, "Timed out");
assert.equal(jobStatusView(job({ status: "completed", conclusion: "action_required" })).label, "Completed: action required");
// A conclusion on a job that is not completed does not change its status.
assert.equal(jobStatusView(job({ status: "in_progress", conclusion: "failure" })).label, "Running");

// Refusals carry the server's reason, and only for refused jobs.
assert.equal(
  refusalText(job({ status: "refused", reason: "Your plan does not include runner size 8x." })),
  "Your plan does not include runner size 8x.",
);
assert.equal(refusalText(job({ status: "refused", reason: "  " })), "It was refused without a recorded reason. Ask your provider.");
assert.equal(refusalText(job({ status: "completed", reason: "stale" })), null);
assert.equal(runsOn("2x"), "shippin-2x");

// Durations and times.
assert.equal(formatDuration(null), "—");
assert.equal(formatDuration(-1), "—");
assert.equal(formatDuration(45), "45 s");
assert.equal(formatDuration(120), "2 min");
assert.equal(formatDuration(185), "3 min 5 s");
assert.equal(formatDuration(3600), "1 h");
assert.equal(formatDuration(3720), "1 h 2 min");
const now = Date.parse("2026-11-15T10:10:00Z");
assert.equal(jobDuration(job({ status: "completed", seconds: 61 }), now), "1 min 1 s");
assert.equal(jobDuration(job({ status: "in_progress", started_at: "2026-11-15T10:05:00Z" }), now), "5 min so far");
assert.equal(jobDuration(job({ status: "queued" }), now), "—");
assert.equal(ago("2026-11-15T10:09:30Z", now), "just now");
assert.equal(ago("2026-11-15T10:00:00Z", now), "10 min ago");
assert.equal(ago("2026-11-15T07:00:00Z", now), "3 h ago");
assert.equal(ago("2026-11-13T10:00:00Z", now), "2 d ago");
assert.equal(ago(null, now), "—");
assert.equal(ago("not a time", now), "—");

// Refreshing and the live summary.
assert.equal(hasActiveJobs([job({ status: "completed" }), job({ status: "refused" })]), false);
assert.equal(hasActiveJobs([job({ status: "in_progress" })]), true);
assert.equal(hasActiveJobs(undefined), false);
assert.equal(
  jobCounts([job({ status: "queued" }), job({ status: "queued" }), job({ status: "in_progress" }), job({ status: "completed" })]),
  "2 waiting, 1 running",
);
assert.equal(jobCounts([job({ status: "refused" })]), "1 refused");
assert.equal(jobCounts([]), "Nothing waiting or running.");

// Repositories.
assert.deepEqual(repositoryState({ workspace_id: null, runners: false }), { label: "Not assigned", tone: "neutral" });
assert.equal(repositoryState({ workspace_id: "w1", runners: true }).label, "Runners allowed");
assert.equal(repositoryState({ workspace_id: "w1", runners: false }).tone, "warning");

// The assignment form refuses what the server would, before sending.
const workspaces = [{ id: "w1" }, { id: "w2" }];
const projects = [
  { id: "p1", workspace_id: "w1" },
  { id: "p2", workspace_id: "w2" },
];
assert.equal(assignmentProblem({ workspaceId: "", projectId: "", runners: true }, workspaces, projects), "Pick a workspace.");
assert.equal(
  assignmentProblem({ workspaceId: "w9", projectId: "", runners: true }, workspaces, projects),
  "Pick one of this organization's workspaces.",
);
assert.equal(
  assignmentProblem({ workspaceId: "w1", projectId: "p2", runners: true }, workspaces, projects),
  "That project is not in the chosen workspace.",
);
assert.equal(assignmentProblem({ workspaceId: "w1", projectId: "p1", runners: false }, workspaces, projects), null);
assert.equal(assignmentProblem({ workspaceId: "w2", projectId: "", runners: true }, workspaces, projects), null);
assert.deepEqual(assignmentBody({ workspaceId: "w1", projectId: "", runners: true }), {
  workspace_id: "w1",
  project_id: null,
  runners: true,
});
assert.deepEqual(assignmentBody({ workspaceId: "w1", projectId: "p1", runners: false }), {
  workspace_id: "w1",
  project_id: "p1",
  runners: false,
});
assert.deepEqual(assignmentFor({ workspace_id: null, project_id: null, runners: false }, "w1"), {
  workspaceId: "w1",
  projectId: "",
  runners: true,
});
assert.deepEqual(assignmentFor({ workspace_id: "w2", project_id: "p2", runners: false }, "w1"), {
  workspaceId: "w2",
  projectId: "p2",
  runners: false,
});

// Errors in plain words.
assert.match(problemText({ status: 503, message: "This install isn't set up for that yet." }), /GitHub isn't set up on this installation/);
assert.match(problemText({ status: 403, message: "" }), /Ask an owner or admin/);
assert.equal(
  problemText({ status: 409, message: "Another organization has already assigned this repository." }),
  "Another organization has already assigned this repository.",
);
assert.equal(problemText(undefined), "That didn't work. Try again in a moment.");

console.log("runner pages model ok");
