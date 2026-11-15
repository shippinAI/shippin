/** How much of a total is used, from 0 to 1. Null when either number is unknown, so an unknown is never drawn as empty. */
export function usedFraction(used: number | null | undefined, total: number | null | undefined): number | null {
  if (used === null || used === undefined || !Number.isFinite(used)) return null;
  if (!total || !Number.isFinite(total) || total <= 0) return null;
  return Math.min(1, Math.max(0, used / total));
}

export type MeterLevel = "normal" | "warning" | "danger";

/** Resource pressure: above three quarters is a warning, above nine tenths is danger. */
export const meterLevel = (fraction: number | null): MeterLevel =>
  fraction === null ? "normal" : fraction > 0.9 ? "danger" : fraction > 0.75 ? "warning" : "normal";
