import type { BypassCert } from "../api/gold-sign-types";

/** "2h 5m", "1m 30s", or "expired". */
export function timeLeft(now: number, exp: number): string {
  const s = exp - now;
  if (s <= 0) return "expired";
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m ${s % 60}s`;
}

/** The header badge text. Always says something: never hidden, never ambiguous. */
export function bypassSummary(active: BypassCert[], now: number, known = true): { label: string; short: string; tone: "neutral" | "danger" | "warning" } {
  if (!known && active.length === 0) return { label: "Bypass status unknown (Gold not synced)", short: "Bypass: unknown", tone: "warning" };
  if (active.length === 0) return { label: "No bypass active", short: "No bypass", tone: "neutral" };
  const soonest = Math.min(...active.map((c) => c.exp));
  const left = timeLeft(now, soonest);
  if (active.length === 1) {
    const c = active[0];
    const scope = c.scope.length > 1 ? `${c.scope[0]} +${c.scope.length - 1}` : (c.scope[0] ?? "no scope");
    return { label: `Bypass active · ${c.holder_agent} · ${scope} · ${left} left`, short: `Bypass ${c.holder_agent} · ${scope} · ${left}`, tone: "danger" };
  }
  const who = [...new Set(active.map((c) => c.holder_agent))].join(", ");
  return { label: `${active.length} bypasses active · ${who} · ${left} left`, short: `${active.length} bypasses · ${left}`, tone: "danger" };
}
