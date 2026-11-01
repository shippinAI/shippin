import { ChevronRight, KeyRound } from "lucide-react";
import { Link } from "react-aria-components";
import { PageHeader } from "../../../template/page-header";
import { Badge } from "../../../lib/design/components/badge/badge";
import { LinkButton } from "../../../lib/design/components/button/link-button";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { useGoldTickets } from "../use-gold-sign";

/** Pending Gold tickets, newest first. Phone-first: one tappable row per ticket. */
export function PendingPage() {
  const q = useGoldTickets();
  const items = q.data?.items ?? [];
  return (
    <>
      <PageHeader eyebrow="Gold change control" title="Gold approvals"
        description="Tickets waiting for your decision. Facts come from gold-policy on Gold, not from the agent."
        actions={<LinkButton href="/gold/passkeys" variant="secondary" tooltip="Register or review your passkeys"><KeyRound className="size-4" aria-hidden /> Passkeys</LinkButton>} />
      {q.data && (
        <p data-testid="sync-status" className={`mb-3 text-xs ${q.data.fresh ? "text-fg-subtle" : "text-warning"}`}>
          {q.data.sync_at === 0 ? "Gold has never synced; this list is empty until it does."
            : q.data.fresh ? `Synced from Gold ${Math.max(0, Math.round(Date.now() / 1000 - q.data.sync_at))}s ago.`
            : `Gold last synced ${new Date(q.data.sync_at * 1000).toLocaleString()}; this list may be out of date.`}
        </p>
      )}
      {q.isError && <EmptyState title="Gold signing is unavailable" description="Only the Gold approver's Nomen account can see this page, and the feature must be configured." />}
      {q.isSuccess && items.length === 0 && <EmptyState title="Nothing to sign" description="No Gold tickets are waiting for a decision." />}
      <ul className="space-y-2" aria-label="Pending tickets">
        {items.map((t) => (
          <li key={t.id}>
            <Link href={`/gold/${encodeURIComponent(t.id)}`}
              className="focus-ring flex min-h-16 items-center gap-3 rounded-xl border border-line bg-surface px-4 py-3 outline-none data-hovered:bg-raised">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-mono text-sm font-semibold text-fg">{t.id}</span>
                  <Badge tone={t.tier === "T2" ? "warning" : "info"}>{t.tier}</Badge>
                  {t.sent ? <Badge tone={t.sent === "approve" ? "success" : "danger"}>{t.sent === "approve" ? "Signed" : "Denied"} · sent to Gold</Badge> : <Badge tone="primary">Needs decision</Badge>}
                </div>
                <div className="mt-1 truncate text-xs text-fg-muted">
                  {t.host} · {(t.classes ?? []).join(", ")} · sha256 <span className="font-mono">{t.sha256.slice(0, 12)}…</span>
                </div>
              </div>
              <ChevronRight className="size-4 shrink-0 text-fg-subtle" aria-hidden />
            </Link>
          </li>
        ))}
      </ul>
    </>
  );
}
