import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet } from "../api/client";
import { jobDone, type Job } from "../api/job";

/** How often a live job is polled (ms); finished jobs aren't polled. */
const JOB_POLL_MS = 2000;

/**
 * Follows a durable job until it finishes. When it does, the queries it changes (given as
 * query keys) are refetched so the UI reflects the result at once.
 */
export function useJob(jobId: string | undefined, refresh: readonly unknown[][] = []) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["job", jobId],
    queryFn: () => apiGet<Job>(`/v1/jobs/${jobId}`),
    enabled: !!jobId,
    refetchInterval: (query) => (jobDone(query.state.data) ? false : JOB_POLL_MS),
  });
  const done = jobDone(q.data);
  useEffect(() => {
    if (done) for (const key of refresh) void qc.invalidateQueries({ queryKey: key });
    // Re-run only when the job finishes (refresh is a fresh array each render).
  }, [done, qc]);
  return q;
}

/** A one-line, user-facing summary of a job that needs attention; undefined while all is well. */
export function jobNotice(j?: Job): string | undefined {
  if (!j) return undefined;
  if (j.status === "failed") return j.error?.detail ?? j.error?.title ?? "The operation failed.";
  if (j.status === "queued" && j.attempts > 0) return `Retrying setup (attempt ${j.attempts + 1})…`;
  return undefined;
}
