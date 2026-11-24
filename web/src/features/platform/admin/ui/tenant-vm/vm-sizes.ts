/** Parses Incus size strings ("32GiB", "150GiB", "20GB") to bytes; 0 when unknown. */
export function sizeBytes(s: string | undefined): number {
  const m = /^(\d+(?:\.\d+)?)\s*([KMGT]i?B)?$/.exec((s ?? "").trim());
  if (!m) return 0;
  const n = Number(m[1]);
  const unit = m[2] ?? "B";
  const pow = { B: 0, KB: 1, KiB: 1, MB: 2, MiB: 2, GB: 3, GiB: 3, TB: 4, TiB: 4 }[unit] ?? 0;
  const base = unit.includes("i") ? 1024 : 1000;
  return Math.round(n * base ** pow);
}
