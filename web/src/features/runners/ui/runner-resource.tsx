import { useQuery } from "@tanstack/react-query";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { Button } from "../../../lib/design/components/button/button";
import { canRead, errorKey, loadResource, type RunnerProject, type Runs, type Connections } from "../model";
export function RunnerResource({ tenant, project, resource }: { tenant: string; project: RunnerProject; resource: "runs" | "connectors" }) {
  const t = useText();
  const allowed = canRead(project, resource);
  const q = useQuery({
    queryKey: ["runner", tenant, project.id, resource, project.scopes],
    queryFn: () => loadResource<Runs | Connections>(tenant, project, resource),
    enabled: allowed, retry: false, gcTime: 0,
  });
  if (!allowed) return <p role="status">{t("runners.denied")}</p>;
  if (q.isError) return <div role="alert"><p>{t(errorKey(q.error))}</p><Button tooltip={t("runners.refresh")} onPress={() => { void q.refetch(); }}>{t("runners.refresh")}</Button></div>;
  if (!q.data || q.isPending) return <p role="status">{t("runners.loading")}</p>;
  if (resource === "runs") {
    const data = q.data as Runs;
    return <div className="space-y-4"><p className="text-fg-muted">{t("runners.eventual")}</p>
      {!(data.items?.length) ? <p>{t("runners.noRuns")}</p> : <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr>{(["runners.id", "runners.repository", "runners.profile", "runners.state", "runners.updated"] as const).map((key) => <th key={key} className="p-3">{t(key)}</th>)}</tr></thead><tbody>{data.items.map((run) => <tr key={run.id} className="border-t border-line"><td className="p-3">{run.id}</td><td className="p-3">{run.repository}</td><td className="p-3">{run.profile}</td><td className="p-3">{run.state}{run.cancel_requested && ` · ${t("runners.cancelling")}`}</td><td className="p-3">{run.projected_at}</td></tr>)}</tbody></table></div>}
      {data.next_cursor && <p>{t("runners.limited")}</p>}
      <Button tooltip={t("runners.refresh")} onPress={() => { void q.refetch(); }}>{t("runners.refresh")}</Button>
    </div>;
  }
  const data = q.data as Connections;
  return <div className="space-y-4">
    <dl className="grid gap-3"><dt>{t("runners.github")}</dt><dd>{data.github.state}</dd><dt>{t("runners.hosts")}</dt><dd>{data.execution_hosts.state}</dd></dl>
    <p className="text-fg-muted">{t("runners.capacityNotice")}</p>
    {!(data.execution_hosts.workers?.length) ? <p>{t("runners.noWorkers")}</p> : <ul className="space-y-3">{data.execution_hosts.workers.map((worker) => <li key={worker.id} className="rounded-lg border border-line p-3"><p>{worker.id} · {worker.state}</p><p className="text-fg-muted">{worker.repository}</p>{worker.last_seen && <p>{t("runners.lastSeen")}: {worker.last_seen}</p>}</li>)}</ul>}
    {data.execution_hosts.truncated && <p>{t("runners.limited")}</p>}
    <Button tooltip={t("runners.refresh")} onPress={() => { void q.refetch(); }}>{t("runners.refresh")}</Button>
  </div>;
}

