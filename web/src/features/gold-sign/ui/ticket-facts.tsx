import type { ReactNode, RefObject } from "react";
import type { GoldFacts } from "../../../lib/api/gold-sign-types";
import { Card } from "../../../lib/design/components/card/card";

function Lines({ label, items }: { label: string; items: string[] | null }) {
  return (
    <div>
      <dt className="text-xs font-medium tracking-wide text-fg-subtle uppercase">{label}</dt>
      <dd className="mt-1">
        {(items ?? []).length === 0 ? <span className="text-sm text-fg-muted">none</span> : (
          <ol className="space-y-1">
            {(items ?? []).map((l, i) => (
              <li key={i} className="rounded-md bg-canvas/70 px-2.5 py-1.5 font-mono text-[12.5px] break-all whitespace-pre-wrap text-fg">{l}</li>
            ))}
          </ol>
        )}
      </dd>
    </div>
  );
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-xs font-medium tracking-wide text-fg-subtle uppercase">{label}</dt>
      <dd className="mt-1 text-sm text-fg">{children}</dd>
    </div>
  );
}

/** Everything here is gold-policy's parse of the ticket. Agent text is shown last and labelled. */
export function TicketFacts({ t, commandsEnd }: { t: GoldFacts; commandsEnd?: RefObject<HTMLDivElement | null> }) {
  return (
    <div className="space-y-4">
      <Card className="p-4 sm:p-5">
        <h2 className="mb-3 text-sm font-semibold text-fg">Facts from gold-policy</h2>
        <dl className="space-y-4">
          <Row label="Ticket sha256 (full, this is what you sign)">
            <code className="block rounded-md bg-canvas/70 px-2.5 py-2 font-mono text-[13px] break-all text-fg" data-testid="sha256">{t.sha256}</code>
          </Row>
          <div className="grid grid-cols-2 gap-4">
            <Row label="Tier">{t.tier}</Row>
            <Row label="Host">{t.host}</Row>
          </div>
          <Lines label="Why this tier" items={t.tier_reasons} />
          <Row label="Classes">{(t.classes ?? []).join(", ") || "none"}</Row>
          <Row label="Out-of-band path">{t.oob_path}</Row>
          <Lines label="Commands (run word for word as root)" items={t.commands} />
          <div ref={commandsEnd} data-testid="commands-end" aria-hidden className="h-px" />
          <Lines label="Rollback" items={t.rollback} />
          <Lines label="Verification" items={t.verification} />
        </dl>
      </Card>
      {t.agent_summary && (
        <Card className="border-dashed p-4 sm:p-5">
          <h2 className="text-sm font-semibold text-fg-muted">Agent summary (agent text, verify against the facts above)</h2>
          <p className="mt-2 text-sm whitespace-pre-wrap text-fg-muted">{t.agent_summary}</p>
        </Card>
      )}
    </div>
  );
}
