import { AlertTriangle } from "lucide-react";
import { jobNotice, useJob } from "../../../lib/hooks/use-job";

/** Shows a retrying or failed devspace provisioning job under the bar; nothing while healthy. */
export function DevspaceJob({ tenantId, jobId }: { tenantId: string; jobId?: string }) {
  const job = useJob(jobId, [["tenant", tenantId, "workspaces"]]);
  const notice = jobNotice(job.data);
  if (!notice) return null;
  const failed = job.data?.status === "failed";
  return (
    <p role="status" className={`flex items-center gap-2 border-b border-line px-4 py-2 text-base ${failed ? "text-danger" : "text-fg-muted"}`}>
      <AlertTriangle className="size-4 shrink-0" aria-hidden />
      {notice}
    </p>
  );
}
