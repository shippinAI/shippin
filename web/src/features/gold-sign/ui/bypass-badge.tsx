import { ShieldAlert, ShieldCheck } from "lucide-react";
import { lazy, Suspense } from "react";
import { Button } from "react-aria-components";
import { Popover } from "../../../lib/design/components/popover/popover";
import { cn } from "../../../lib/design/components/cn/cn";
import { bypassSummary } from "../../../lib/gold-sign/bypass-model";
import { useBypass } from "../use-gold-sign";

// Details (revoke flow, WebAuthn) load on first open so the always-present badge stays tiny.
const BypassDetails = lazy(() => import("./bypass-details").then((m) => ({ default: m.BypassDetails })));

/** Persistent header badge: always states whether a bypass certificate is active. */
export function BypassBadge() {
  const q = useBypass();
  const active = q.data?.status.active ?? [];
  const now = q.data?.status.now ?? Math.floor(Date.now() / 1000);
  const s = q.isError ? { label: "Bypass status unavailable", short: "Bypass: unknown", tone: "warning" as const } : bypassSummary(active, now, q.data?.status.known ?? false);
  const Icon = s.tone === "neutral" ? ShieldCheck : ShieldAlert;
  const tone = { neutral: "border-line bg-raised text-fg-muted", danger: "border-danger/40 bg-danger/15 text-danger", warning: "border-warning/40 bg-warning/15 text-warning" }[s.tone];
  return (
    <Popover title={active.length ? "Active bypass certificates" : "Bypass certificates"} placement="bottom end"
      trigger={
        <Button aria-label={s.label} data-testid="bypass-badge"
          className={cn("focus-ring inline-flex h-8 max-w-[68vw] items-center gap-1.5 rounded-full border px-2.5 text-xs font-medium outline-none", tone)}>
          <Icon className="size-3.5 shrink-0" aria-hidden />
          <span className="truncate sm:hidden">{s.short}</span>
          <span className="hidden truncate sm:inline">{s.label}</span>
        </Button>
      }>
      {q.data ? <Suspense fallback={<p>Loading…</p>}><BypassDetails info={q.data} /></Suspense> : <p>{q.isError ? "Could not load bypass status. Treat it as unknown." : "Loading…"}</p>}
    </Popover>
  );
}
