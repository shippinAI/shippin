/** A durable job (OpenAPI Job): GET /v1/jobs/{id}. */
export type JobStatus = "queued" | "running" | "succeeded" | "failed" | "canceling" | "canceled";

export type Job = {
  id: string;
  tenant_id: string;
  kind: string;
  target: { type: string; id: string };
  status: JobStatus;
  /** The current step, e.g. "Starting". */
  message?: string;
  attempts: number;
  /** Set when the job failed; detail is shown to operators only. */
  error?: { title: string; detail?: string };
  created_by: string;
  created_at: string;
  finished_at?: string;
};

export const jobDone = (j?: Job) => j?.status === "succeeded" || j?.status === "failed" || j?.status === "canceled";
