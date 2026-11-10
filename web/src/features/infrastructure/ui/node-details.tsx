import { Badge } from "../../../lib/design/components/badge/badge";
import { Card } from "../../../lib/design/components/card/card";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import { formatBytes } from "../../../lib/format";
import type { InfraNode } from "../../../lib/hooks/use-infra";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { formatUptime, memoryPercent } from "../model";

function Fact({ label, value, wide }: { label: string; value?: string; wide?: boolean }) {
  if (!value) return null;
  return (
    <div className={wide ? "sm:col-span-2" : undefined}>
      <dt className="text-xs text-fg-subtle">{label}</dt>
      <dd className="mt-0.5 break-words text-base text-fg">{value}</dd>
    </div>
  );
}

/** What this host says about itself, read live. Anything it did not report is simply absent. */
export function NodeFacts({ node }: { node: InfraNode }) {
  const t = useText();
  const d = node.details;
  const memory = memoryPercent(d);
  const cpu = d?.cpu_model ? `${d.cpu_model}${d.cpus ? ` · ${d.cpus}` : ""}` : d?.cpus ? String(d.cpus) : undefined;
  return (
    <Card className="mb-6 p-5" data-testid="node-facts">
      <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Fact label={t("infra.fact.version")} value={node.version} />
        <Fact label={t("infra.fact.release")} value={d?.release} wide />
        <Fact label={t("infra.fact.os")} value={d?.os} />
        <Fact label={t("infra.fact.kernel")} value={d?.kernel} />
        <Fact label={t("infra.fact.cpu")} value={cpu} wide />
        <Fact label={t("infra.fact.uptime")} value={d?.uptime_seconds ? formatUptime(d.uptime_seconds) : undefined} />
        <Fact label={t("infra.fact.project")} value={node.scope} />
      </dl>
      {memory !== null && (
        <div className="mt-4">
          <div className="flex justify-between text-xs text-fg-subtle">
            <span>{t("infra.fact.memory")}</span>
            <span className="tabular-nums">{formatBytes(d!.mem_used_bytes ?? 0)} / {formatBytes(d!.mem_total_bytes ?? 0)}</span>
          </div>
          <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-raised">
            <div className="h-full rounded-full bg-primary" style={{ width: `${memory}%` }} />
          </div>
        </div>
      )}
      {d?.members && d.members.length > 0 && (
        <div className="mt-4" data-testid="native-cluster">
          <p className="text-xs text-fg-subtle">{d.cluster ? t("infra.fact.clusterNamed", { name: d.cluster }) : t("infra.fact.cluster")}</p>
          <ul className="mt-1 flex flex-wrap gap-2">
            {d.members.map((m) => (
              <li key={m.name}>
                <Badge tone={m.online ? "success" : "danger"}><StatusDot health={m.online ? "up" : "down"} /> {m.name}{m.local ? ` · ${t("infra.fact.thisNode")}` : ""}</Badge>
              </li>
            ))}
          </ul>
        </div>
      )}
      {d?.quorate !== undefined && (
        <div className="mt-4" data-testid="cluster-quorum">
          <span className="text-xs text-fg-subtle">{t("infra.fact.quorum")}: </span>
          <Badge tone={d.quorate ? "success" : "danger"}><StatusDot health={d.quorate ? "up" : "down"} /> {t(d.quorate ? "infra.fact.quorate" : "infra.fact.notQuorate")}</Badge>
        </div>
      )}
      {d?.replication_status_available !== undefined && (
        <section className="mt-5" data-testid="replication-jobs" aria-label={t("infra.fact.replication")}>
          <h3 className="text-base font-medium text-fg">{t("infra.fact.replication")}</h3>
          {!d.replication_status_available ? <p className="mt-2 text-base text-fg-muted">{t("infra.fact.replicationUnavailable")}</p> : d.replication_jobs?.length ? (
            <ul className="mt-2 divide-y divide-line rounded-md border border-line">
              {d.replication_jobs.map((job) => <li key={job.id} className="grid gap-1 px-3 py-2 text-base sm:grid-cols-2">
                <span className="font-mono text-fg">{job.id} · {t("infra.fact.guest")} {job.guest}</span>
                <span className="text-fg-muted">{job.target ? `${t("infra.fact.target")}: ${job.target}` : ""}{job.schedule ? ` · ${job.schedule}` : ""}{job.running ? ` · ${t("infra.fact.running")}` : ""}</span>
                {job.fail_count ? <span className="text-danger">{t("infra.fact.replicationFailures", { count: job.fail_count })}{job.error ? ` · ${job.error}` : ""}</span> : null}
              </li>)}
            </ul>
          ) : <p className="mt-2 text-base text-fg-muted">{t("infra.fact.noReplicationJobs")}</p>}
        </section>
      )}
      {d?.disks && d.disks.length > 0 && (
        <div className="mt-5" data-testid="node-disks">
          <h3 className="text-base font-medium text-fg">{t("infra.fact.disks")}</h3>
          <ul className="mt-2 divide-y divide-line rounded-md border border-line">
            {d.disks.map((disk) => (
              <li key={disk.path} className="grid gap-2 px-3 py-2 text-base sm:grid-cols-2 lg:grid-cols-4">
                <div className="min-w-0"><dt className="text-xs text-fg-subtle">{t("infra.fact.diskModel")}</dt><dd className="break-words text-fg">{disk.model || disk.path}</dd>{disk.model && <dd className="break-words text-xs text-fg-muted">{disk.path}</dd>}</div>
                <div><dt className="text-xs text-fg-subtle">{t("infra.fact.diskCapacity")}</dt><dd className="text-fg">{formatBytes(disk.size_bytes)}</dd></div>
                <div><dt className="text-xs text-fg-subtle">{t("infra.fact.diskUsage")}</dt><dd className="text-fg">{disk.used || "—"}</dd></div>
                <div className="flex gap-3 text-xs text-fg-muted">
                  <span>{t("infra.fact.diskMounted")}: {t(disk.mounted ? "infra.fact.yes" : "infra.fact.no")}</span>
                  <span>{t("infra.fact.diskGPT")}: {t(disk.gpt ? "infra.fact.yes" : "infra.fact.no")}</span>
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Card>
  );
}
