import { Badge, type BadgeTone } from "../../../lib/design/components/badge/badge";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { useOperations, type OperationStatus } from "../../../lib/hooks/use-infra";
import { useText } from "../../../lib/ui-config/use-ui-config";
import type { TextKey } from "../../../lib/ui-config/text-keys";

const tones: Record<OperationStatus, BadgeTone> = { queued: "neutral", running: "info", succeeded: "success", failed: "danger", unknown: "warning", skipped: "neutral" };
const words: Record<OperationStatus, TextKey> = {
  queued: "infra.op.queued", running: "infra.op.running", succeeded: "infra.op.succeeded", failed: "infra.op.failed", unknown: "infra.op.unknown", skipped: "infra.op.skipped",
};

/** Every change to a guest and how it ended. It comes from the server, so it survives leaving the page. */
export function ActivityCard({ serverId, showHost }: { serverId?: string; showHost?: boolean }) {
  const t = useText();
  const q = useOperations(serverId);
  const items = q.data?.items ?? [];
  return (
    <Card className="overflow-hidden" data-testid="activity">
      <CardHeader title={t("infra.activity.title")} description={t("infra.activity.description")} />
      {items.length === 0 ? (
        <p className="p-5 text-sm text-fg-muted">{q.isError ? q.error.message : t("infra.activity.empty")}</p>
      ) : (
        <ul className="divide-y divide-line">
          {items.map((o) => (
            <li key={o.id} className="px-5 py-3 text-sm" data-testid="operation">
              <div className="flex flex-wrap items-center gap-2">
                <Badge tone={tones[o.status]}>{t(words[o.status])}</Badge>
                <span className="text-fg"><span className="font-medium">{o.verb}</span> <span className="font-mono text-[12.5px]">{o.guest}</span></span>
                {showHost && o.server_label && <span className="text-fg-muted">{t("infra.activity.on", { host: o.server_label })}</span>}
                {o.guest_status && o.status === "succeeded" && <span className="text-fg-subtle">{t("infra.activity.nowStatus", { status: o.guest_status })}</span>}
                <span className="ml-auto text-[12px] text-fg-subtle">{o.actor_name ? `${o.actor_name} · ` : ""}{new Date(o.started_at).toLocaleString()}</span>
              </div>
              {o.note && <p className="mt-1 break-words text-[12.5px] text-fg-muted" data-testid="operation-note">{t("infra.activity.why", { note: o.note })}</p>}
              {o.detail && <p className={`mt-1 break-words text-[12.5px] ${o.status === "failed" ? "text-danger" : o.status === "unknown" ? "text-warning" : "text-fg-muted"}`}>{o.detail}</p>}
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}
