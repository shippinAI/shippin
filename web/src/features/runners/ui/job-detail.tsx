import type { ReactNode } from "react";
import { Drawer } from "../../../lib/design/components/drawer/drawer";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { useProjects } from "../../../lib/hooks/use-projects";
import type { OrganizationWorkspace } from "../../workspaces/use-organization-workspaces";
import { ago, jobDuration, jobStatusView, problemText, runsOn } from "../model";
import { useRunnerJob } from "../use-runners";
import { JobStatusBadge, Refusal } from "./job-status";

const when = (iso: string | null) => (iso ? new Date(iso).toLocaleString() : "—");

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[9rem_1fr] gap-3 border-b border-line py-2.5 text-base last:border-b-0">
      <dt className="text-fg-subtle">{label}</dt>
      <dd className="min-w-0 break-words text-fg">{children}</dd>
    </div>
  );
}

type Props = { tenantId: string; jobId?: string; workspaces: OrganizationWorkspace[]; onClose: () => void };

/** One job read fresh with shippin.runtime.runner_job.get: where it came from, what it asked for and what happened. */
export function JobDetail({ tenantId, jobId, workspaces, onClose }: Props) {
  const q = useRunnerJob(tenantId, jobId);
  const projects = useProjects(tenantId);
  const j = q.data;
  const now = q.dataUpdatedAt || Date.now();
  const ws = workspaces.find((w) => w.id === j?.workspace_id)?.name;
  const project = projects.data?.find((p) => p.id === j?.project_id)?.name;
  return (
    <Drawer
      isOpen={!!jobId}
      onOpenChange={(o) => {
        if (!o) onClose();
      }}
      title={j?.name || "Runner job"}
      subtitle={j?.repository}
    >
      <div className="space-y-5 px-6 py-5">
        {q.isPending && <Skeleton className="h-48" />}
        {q.isError && (
          <p role="alert" className="text-base text-danger">
            {problemText(q.error)}
          </p>
        )}
        {j && (
          <>
            <div className="flex items-center gap-3">
              <JobStatusBadge job={j} />
              <span className="text-sm text-fg-muted">{jobStatusView(j).hint}</span>
            </div>
            <Refusal job={j} />
            <dl>
              <Fact label="Repository">
                <span className="font-mono text-sm">{j.repository}</span>
              </Fact>
              <Fact label="Workspace">
                {j.workspace_id ? (ws ?? "A removed workspace") : <span className="text-fg-subtle">Not assigned when the job came</span>}
              </Fact>
              {j.project_id && <Fact label="Project">{project ?? "A removed project"}</Fact>}
              <Fact label="Size">{j.size ? <span className="font-mono text-sm">{runsOn(j.size)}</span> : "—"}</Fact>
              <Fact label="Labels">
                <span className="font-mono text-sm">{j.labels.join(", ") || "—"}</span>
              </Fact>
              <Fact label="Queued">
                <time dateTime={j.queued_at}>{when(j.queued_at)}</time> <span className="text-fg-subtle">({ago(j.queued_at, now)})</span>
              </Fact>
              <Fact label="Started">{when(j.started_at)}</Fact>
              <Fact label="Finished">{when(j.completed_at)}</Fact>
              <Fact label="Duration">{jobDuration(j, now)}</Fact>
              {j.conclusion && <Fact label="GitHub's result">{j.conclusion.replace(/_/g, " ")}</Fact>}
              {j.runner_name && (
                <Fact label="Runner">
                  <span className="font-mono text-sm">{j.runner_name}</span>
                </Fact>
              )}
              <Fact label="GitHub run">
                <span className="font-mono text-sm">{j.run_id}</span>
              </Fact>
              <Fact label="GitHub job">
                <span className="font-mono text-sm">{j.github_job_id}</span>
              </Fact>
            </dl>
            <p className="text-sm text-fg-subtle">Logs are not kept here yet; open the run on GitHub for its output.</p>
          </>
        )}
      </div>
    </Drawer>
  );
}
