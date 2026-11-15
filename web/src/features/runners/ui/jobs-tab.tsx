import { useState } from "react";
import type { RunnerJob } from "../../../lib/api/actions.gen";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { Select } from "../../../lib/design/components/select/select";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { useOrganizationWorkspaces } from "../../workspaces/use-organization-workspaces";
import { ago, jobCounts, jobDuration, problemText, runsOn } from "../model";
import { useRunnerJobs } from "../use-runners";
import { JobDetail } from "./job-detail";
import { JobStatusBadge, Refusal } from "./job-status";

const allWorkspaces = "__all";

/** The organization's runner jobs, newest first: waiting, refused (with why), running and finished. */
export function JobsTab({ tenantId }: { tenantId: string }) {
  const [workspace, setWorkspace] = useState("");
  const [open, setOpen] = useState<string>();
  const workspaces = useOrganizationWorkspaces(tenantId);
  const q = useRunnerJobs(tenantId, workspace);
  const now = q.dataUpdatedAt || Date.now();
  const jobs = q.data?.items ?? [];
  const columns: ColumnDef<RunnerJob>[] = [
    { id: "job", label: "Job", isRowHeader: true, render: (j) => (
      <div className="min-w-0">
        <div className="truncate text-fg">{j.name || "Unnamed job"}</div>
        <div className="truncate font-mono text-xs text-fg-subtle">{j.repository}</div>
        <Refusal job={j} compact />
      </div>
    ) },
    { id: "size", label: "Size", width: "9rem", render: (j) => j.size ? <span className="font-mono text-sm">{runsOn(j.size)}</span> : <span className="text-fg-subtle">—</span> },
    { id: "status", label: "Status", width: "9rem", render: (j) => <JobStatusBadge job={j} /> },
    { id: "queued", label: "Queued", width: "8rem", render: (j) => <time dateTime={j.queued_at} title={new Date(j.queued_at).toLocaleString()}>{ago(j.queued_at, now)}</time> },
    { id: "duration", label: "Duration", width: "8rem", render: (j) => jobDuration(j, now) },
  ];
  return (
    <>
      <Card className="overflow-hidden">
        <CardHeader title="Jobs" description="Every GitHub Actions job that asked for a Shippin runner, newest first. Open one for its details."
          actions={<Button size="sm" variant="ghost" tooltip="Load the latest jobs now" isDisabled={q.isFetching} onPress={() => void q.refetch()}>Refresh</Button>} />
        <div className="flex flex-wrap items-end justify-between gap-4 px-5 py-4">
          <div className="w-64">
            <Select label="Workspace" value={workspace || allWorkspaces} onChange={(v) => setWorkspace(v === allWorkspaces ? "" : v)}
              options={[{ id: allWorkspaces, label: "All workspaces" }, ...(workspaces.data ?? []).map((w) => ({ id: w.id, label: w.name }))]} />
          </div>
          {q.data && <p aria-live="polite" className="text-sm text-fg-muted">{jobCounts(jobs)}</p>}
        </div>
        {q.isPending && <div className="px-5 pb-5"><Skeleton className="h-32" /></div>}
        {q.isError && <p role="alert" className="px-5 pb-5 text-base text-danger">{problemText(q.error)}</p>}
        {q.data && (
          <DataTable label="Runner jobs" columns={columns} rows={jobs} dense dependencies={[now]} onRowAction={setOpen}
            empty={<p className="px-5 py-8 text-center text-base text-fg-muted">{workspace ? "No jobs from this workspace's repositories yet." : "No jobs yet. A job appears here when a workflow in an assigned repository asks for a Shippin runner."}</p>} />
        )}
      </Card>
      <JobDetail tenantId={tenantId} jobId={open} workspaces={workspaces.data ?? []} onClose={() => setOpen(undefined)} />
    </>
  );
}
