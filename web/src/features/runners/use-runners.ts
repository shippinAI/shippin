import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { callAction } from "../../lib/api/actions";
import { toast } from "../../lib/design/components/toast/toast-queue";
import { hasActiveJobs, problemText } from "./model";

const key = (tenantId: string) => ["tenant", tenantId, "runners"];

/** The organization's GitHub repositories, as its GitHub connections reach them, with where each is assigned. */
export const useRunnerRepositories = (tenantId: string) =>
  useQuery({
    queryKey: [...key(tenantId), "repositories"],
    queryFn: () => callAction("shippin.integrations.github_repository.list", { tenantId }),
    enabled: !!tenantId,
    retry: false,
  });

/** The organization's runner jobs, newest first, optionally one workspace's. Refreshes while any is waiting or running. */
export const useRunnerJobs = (tenantId: string, workspace: string) =>
  useQuery({
    queryKey: [...key(tenantId), "jobs", workspace || "all"],
    queryFn: () => callAction("shippin.runtime.runner_job.list", { tenantId, workspace: workspace || undefined }),
    enabled: !!tenantId,
    retry: false,
    refetchInterval: (q) => (hasActiveJobs(q.state.data?.items) ? 15_000 : false),
  });

/** One job, for its detail view. */
export const useRunnerJob = (tenantId: string, runnerJobId: string | undefined) =>
  useQuery({
    queryKey: [...key(tenantId), "job", runnerJobId],
    queryFn: () => callAction("shippin.runtime.runner_job.get", { tenantId, runnerJobId: runnerJobId ?? "" }),
    enabled: !!tenantId && !!runnerJobId,
    retry: false,
    refetchInterval: (q) => (hasActiveJobs(q.state.data ? [q.state.data] : undefined) ? 15_000 : false),
  });

/** One mutation for repository changes made through actions (callAction); refreshes the runner queries. */
export function useRunnerChange(tenantId: string, done: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (run: () => Promise<unknown>) => run(),
    onSuccess: () => toast.success(done),
    onError: (e) => toast.error("Not saved", problemText(e)),
    onSettled: () => void qc.invalidateQueries({ queryKey: key(tenantId) }),
  });
}
