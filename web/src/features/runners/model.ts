// What the organization's build-runner pages show (RUN-01, docs/runners.md), kept apart from React so
// it is tested on its own. Every type here is the generated one from the action's Go types.
import type { GithubRepository, GithubRepositoryUpdateInput, RunnerJob, RunnerJobStatus } from "../../lib/api/actions.gen";
import type { BadgeTone } from "../../lib/design/components/badge/badge";

/** The icon a status shows beside its words; the page maps each to a lucide icon. */
export type StatusIcon = "clock" | "ban" | "play" | "check" | "x" | "minus";

export type StatusView = { label: string; tone: BadgeTone; icon: StatusIcon; hint: string };

const statuses: Record<RunnerJobStatus, StatusView> = {
  queued: { label: "Waiting", tone: "info", icon: "clock", hint: "Recorded and waiting for a runner." },
  refused: { label: "Refused", tone: "danger", icon: "ban", hint: "No runner was started for this job. The reason says what to change." },
  in_progress: { label: "Running", tone: "primary", icon: "play", hint: "Running on its own runner." },
  completed: { label: "Completed", tone: "success", icon: "check", hint: "Finished; its runner is removed." },
  canceled: { label: "Canceled", tone: "neutral", icon: "minus", hint: "Canceled before it finished." },
};

/** GitHub's conclusions for a finished job, in words. */
const conclusions: Record<string, { label: string; tone: BadgeTone; icon: StatusIcon }> = {
  success: { label: "Succeeded", tone: "success", icon: "check" },
  failure: { label: "Failed", tone: "danger", icon: "x" },
  timed_out: { label: "Timed out", tone: "danger", icon: "x" },
  cancelled: { label: "Canceled", tone: "neutral", icon: "minus" },
  skipped: { label: "Skipped", tone: "neutral", icon: "minus" },
};

/** How a job's status reads: words first, a tone and an icon beside them, never colour alone. A completed
 * job reads as its GitHub conclusion (succeeded, failed) when there is one. */
export function jobStatusView(job: Pick<RunnerJob, "status" | "conclusion">): StatusView {
  const base = statuses[job.status] ?? { label: job.status, tone: "neutral", icon: "minus", hint: "" };
  if (job.status !== "completed" || !job.conclusion) return base;
  const c = conclusions[job.conclusion];
  return c ? { ...base, ...c } : { ...base, label: `Completed: ${job.conclusion.replace(/_/g, " ")}` };
}

/** Why the job got no runner, for a refused job; null for any other. */
export function refusalText(job: Pick<RunnerJob, "status" | "reason">): string | null {
  if (job.status !== "refused") return null;
  return job.reason.trim() || "It was refused without a recorded reason. Ask your provider.";
}

/** The label a workflow asks for a size with. */
export const runsOn = (size: string) => `shippin-${size}`;

/** "45 s", "3 min 5 s", "1 h 2 min". */
export function formatDuration(seconds: number | null | undefined): string {
  if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return "—";
  const s = Math.round(seconds);
  if (s < 60) return `${s} s`;
  if (s < 3600) return s % 60 ? `${Math.floor(s / 60)} min ${s % 60} s` : `${s / 60} min`;
  const m = Math.floor((s % 3600) / 60);
  return m ? `${Math.floor(s / 3600)} h ${m} min` : `${Math.floor(s / 3600)} h`;
}

/** How long a job ran, or has been running so far. */
export function jobDuration(job: Pick<RunnerJob, "status" | "seconds" | "started_at">, now: number): string {
  if (job.seconds != null) return formatDuration(job.seconds);
  if (job.status === "in_progress" && job.started_at) {
    const t = Date.parse(job.started_at);
    if (Number.isFinite(t)) return `${formatDuration(Math.max(0, (now - t) / 1000))} so far`;
  }
  return "—";
}

/** "just now", "5 min ago", "3 h ago", "2 d ago". */
export function ago(iso: string | null | undefined, now: number): string {
  if (!iso) return "—";
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return "—";
  const s = Math.max(0, Math.round((now - t) / 1000));
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  return `${Math.floor(s / 86400)} d ago`;
}

/** Whether the list should keep refreshing: something is still waiting or running. */
export const hasActiveJobs = (jobs: Pick<RunnerJob, "status">[] | undefined) =>
  !!jobs?.some((j) => j.status === "queued" || j.status === "in_progress");

/** "2 waiting, 1 running", for the polite live region above the list. */
export function jobCounts(jobs: Pick<RunnerJob, "status">[]): string {
  const n = (s: RunnerJobStatus) => jobs.filter((j) => j.status === s).length;
  const parts = [
    [n("queued"), "waiting"],
    [n("in_progress"), "running"],
    [n("refused"), "refused"],
  ] as const;
  const said = parts.filter(([c]) => c > 0).map(([c, w]) => `${c} ${w}`);
  return said.length ? said.join(", ") : "Nothing waiting or running.";
}

/** Where a repository stands for runners. */
export type RepositoryState = { label: string; tone: BadgeTone };
export function repositoryState(r: Pick<GithubRepository, "workspace_id" | "runners">): RepositoryState {
  if (!r.workspace_id) return { label: "Not assigned", tone: "neutral" };
  return r.runners ? { label: "Runners allowed", tone: "success" } : { label: "Runners off", tone: "warning" };
}

/** The assignment form's values. projectId "" is the whole workspace. */
export type Assignment = { workspaceId: string; projectId: string; runners: boolean };

/** What is wrong with an assignment before it is sent, in the words the server would use; null when it may go. */
export function assignmentProblem(
  a: Assignment,
  workspaces: { id: string }[],
  projects: { id: string; workspace_id: string }[],
): string | null {
  if (!a.workspaceId) return "Pick a workspace.";
  if (!workspaces.some((w) => w.id === a.workspaceId)) return "Pick one of this organization's workspaces.";
  if (a.projectId && !projects.some((p) => p.id === a.projectId && p.workspace_id === a.workspaceId))
    return "That project is not in the chosen workspace.";
  return null;
}

/** The update action's body for an assignment. */
export const assignmentBody = (a: Assignment): GithubRepositoryUpdateInput => ({
  workspace_id: a.workspaceId,
  project_id: a.projectId || null,
  runners: a.runners,
});

/** The form's starting values for a repository: where it is now, or nothing yet with runners allowed. */
export const assignmentFor = (r: Pick<GithubRepository, "workspace_id" | "project_id" | "runners">, defaultWorkspace = ""): Assignment =>
  r.workspace_id
    ? { workspaceId: r.workspace_id, projectId: r.project_id ?? "", runners: r.runners }
    : { workspaceId: defaultWorkspace, projectId: "", runners: true };

/** An error from a runner action, in plain words a member can act on. */
export function problemText(e: unknown): string {
  const err = (e ?? {}) as { status?: number; message?: string };
  if (err.status === 503) return "GitHub isn't set up on this installation yet. Ask your provider to configure the Shippin GitHub App.";
  if (err.status === 403) return "Your role in this organization doesn't include that. Ask an owner or admin.";
  const m = err.message?.trim();
  return m || "That didn't work. Try again in a moment.";
}
