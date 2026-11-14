import type { CoverageTierCustomerView } from "../../../lib/api/actions.gen";
import type { ReactNode } from "react";
import type { CoverageTier } from "../../../lib/api/catalog-types";
import { Badge } from "../../../lib/design/components/badge/badge";
import { channelLabel, clockFor, operationsLabel, reviewsLabel, services, severities, supportHoursLabel, target, termLabel } from "../wording";

type Shown = CoverageTierCustomerView & Partial<CoverageTier>;

/** What a coverage tier includes, as a definition list. Compact drops the services and extras. */
export function CoverageSummary({ tier: c, compact = false }: { tier: Shown; compact?: boolean }) {
  const row = (term: string, value: ReactNode) => (
    <div className="flex gap-3 py-1.5"><dt className="w-36 shrink-0 text-fg-subtle">{term}</dt><dd className="min-w-0 text-fg">{value}</dd></div>
  );
  return (
    <dl className="divide-y divide-line text-sm">
      {row("Support hours", supportHoursLabel[c.support_hours])}
      {row("First response", <ul className="space-y-0.5">{severities.map((s) => (
        <li key={s.key} title={s.hint}><span className="text-fg-muted">{s.label}:</span> {target(c[s.key], clockFor(c.support_hours, s.key))}</li>
      ))}</ul>)}
      {row("Uptime target", c.uptime_target ? `${c.uptime_target}%` : "None")}
      {row("Operations", operationsLabel[c.operations])}
      {!compact && row("Channels", <span className="flex flex-wrap gap-1">{c.channels.map((ch) => <Badge key={ch}>{channelLabel[ch]}</Badge>)}</span>)}
      {!compact && row("Named contact", c.named_contact ? "Yes" : "No")}
      {!compact && row("Reviews", reviewsLabel[c.reviews])}
      {!compact && row("Training", c.training_hours ? `${c.training_hours} hours a year` : "None included")}
      {!compact && services.map((s) => row(s.label, termLabel[c[s.key]]))}
      {compact && row("Services", <span className="flex flex-wrap gap-1">{services.filter((s) => c[s.key] === "included").map((s) => <Badge key={s.key} tone="success">{s.label}</Badge>)}
        {services.every((s) => c[s.key] !== "included") && <span className="text-fg-muted">Quoted on request</span>}</span>)}
    </dl>
  );
}
