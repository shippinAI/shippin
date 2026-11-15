// The maths behind the time-series chart: scales, ticks, unit formatting, and lines that break where data is missing.
// Pure and DOM-free so it can be tested on its own. A missing reading is null and stays missing: nothing here
// interpolates across it or reads it as zero (OBS-01).

export type ChartUnit = "fraction" | "bytes" | "bytes_per_second" | "count";
/** One reading. `at` is milliseconds since the epoch; null means nothing was sampled then. */
export type ChartPoint = { at: number; value: number | null };
export type Domain = readonly [number, number];

export type Scale = ((v: number) => number) & { invert: (p: number) => number };

/** Maps a domain onto a range linearly. An empty domain or range maps everything to its start. */
export function linearScale([d0, d1]: Domain, [r0, r1]: Domain): Scale {
  const dSpan = d1 - d0, rSpan = r1 - r0;
  const f = ((v: number) => (dSpan === 0 ? r0 : r0 + ((v - d0) / dSpan) * rSpan)) as Scale;
  f.invert = (p: number) => (rSpan === 0 ? d0 : d0 + ((p - r0) / rSpan) * dSpan);
  return f;
}

const clean = (n: number) => Number(n.toPrecision(12));

/** 1, 2, 2.5 or 5 times a power of ten: the smallest such step at least `rough`. */
export function niceStep(rough: number): number {
  if (!(rough > 0) || !Number.isFinite(rough)) return 1;
  const p = 10 ** Math.floor(Math.log10(rough));
  const m = rough / p;
  return clean((m <= 1 ? 1 : m <= 2 ? 2 : m <= 2.5 ? 2.5 : m <= 5 ? 5 : 10) * p);
}

const KIB = 1024;
const byteUnits = ["B", "KB", "MB", "GB", "TB", "PB"];
const byteExponent = (n: number) => (n < KIB ? 0 : Math.min(byteUnits.length - 1, Math.floor(Math.log(n) / Math.log(KIB))));

/**
 * Value-axis ticks from `min` to at least `max`, about `count` of them, and the axis top they reach.
 * Bytes step in binary units (2 GB, not 2.15 GB) and a fraction steps in whole percents.
 */
export function valueTicks(min: number, max: number, unit: ChartUnit, count = 4): { ticks: number[]; top: number } {
  if (!(max > min)) max = min + 1;
  const base = unit === "bytes" || unit === "bytes_per_second" ? KIB ** byteExponent(Math.max(Math.abs(max), Math.abs(min))) : 1;
  const step = niceStep((max - min) / base / Math.max(1, count)) * base;
  const first = Math.floor(min / step), last = Math.ceil(clean(max / step));
  const ticks: number[] = [];
  for (let k = first; k <= last; k++) ticks.push(clean(k * step));
  return { ticks, top: ticks[ticks.length - 1]! };
}

/** The value domain for these readings: from zero (or a negative reading) to the largest reading, or `ceiling` if higher. */
export function valueDomain(values: (number | null)[], unit: ChartUnit, ceiling?: number): Domain {
  let lo = 0, hi = unit === "fraction" ? 1 : 0;
  for (const v of values) if (v !== null && Number.isFinite(v)) { lo = Math.min(lo, v); hi = Math.max(hi, v); }
  if (ceiling !== undefined && ceiling > hi) hi = ceiling;
  return [lo, hi > lo ? hi : lo + 1];
}

const MIN = 60_000, HOUR = 60 * MIN, DAY = 24 * HOUR;
const timeSteps = [MIN, 2 * MIN, 5 * MIN, 10 * MIN, 15 * MIN, 30 * MIN, HOUR, 2 * HOUR, 3 * HOUR, 6 * HOUR, 12 * HOUR, DAY, 2 * DAY, 7 * DAY, 14 * DAY, 30 * DAY];

/**
 * Time-axis ticks on round local times between t0 and t1, at most `maxCount` of them.
 * `offsetMs` is local time minus UTC (minutes east times 60 000), so a day tick lands on local midnight.
 */
export function timeTicks(t0: number, t1: number, maxCount: number, offsetMs = 0): { ticks: number[]; step: number } {
  const span = Math.max(0, t1 - t0);
  const step = timeSteps.find((s) => span / s <= Math.max(1, maxCount)) ?? timeSteps[timeSteps.length - 1]!;
  const ticks: number[] = [];
  for (let t = Math.ceil((t0 + offsetMs) / step) * step - offsetMs; t <= t1; t += step) ticks.push(t);
  return { ticks, step };
}

/** Whether a tick at this step reads as a time of day or as a date. */
export const timeTickKind = (step: number): "time" | "date" => (step >= DAY ? "date" : "time");

/** The usual spacing of the readings: the median gap between consecutive timestamps, or null with fewer than two. */
export function sampleStep(points: ChartPoint[]): number | null {
  const gaps: number[] = [];
  for (let i = 1; i < points.length; i++) {
    const d = points[i]!.at - points[i - 1]!.at;
    if (d > 0) gaps.push(d);
  }
  if (!gaps.length) return null;
  gaps.sort((a, b) => a - b);
  return gaps[Math.floor(gaps.length / 2)]!;
}

/**
 * The runs of consecutive readings that can be joined by a line. A run ends at a null reading, and at a jump in
 * time longer than `maxGapMs` (rows the sampler never wrote). Defaults to one and a half usual sample steps.
 */
export function segments(points: ChartPoint[], maxGapMs?: number): ChartPoint[][] {
  const sorted = [...points].filter((p) => Number.isFinite(p.at)).sort((a, b) => a.at - b.at);
  const step = sampleStep(sorted);
  const limit = maxGapMs ?? (step === null ? Infinity : step * 1.5);
  const runs: ChartPoint[][] = [];
  let run: ChartPoint[] = [];
  for (const p of sorted) {
    const ok = p.value !== null && Number.isFinite(p.value);
    const prev = run[run.length - 1];
    if (!ok || (prev && p.at - prev.at > limit)) {
      if (run.length) runs.push(run);
      run = [];
    }
    if (ok) run.push(p);
  }
  if (run.length) runs.push(run);
  return runs;
}

const r1 = (n: number) => Math.round(n * 10) / 10;

/** An SVG path through one run. A run of one reading is a single move, which the chart draws as a dot. */
export function linePath(run: ChartPoint[], x: (t: number) => number, y: (v: number) => number): string {
  return run.map((p, i) => `${i ? "L" : "M"}${r1(x(p.at))},${r1(y(p.value!))}`).join("");
}

/** An SVG area under one run, closed down to the baseline. Empty for a run of one reading. */
export function areaPath(run: ChartPoint[], x: (t: number) => number, y: (v: number) => number, baseline: number): string {
  if (run.length < 2) return "";
  const first = run[0]!, last = run[run.length - 1]!;
  return `M${r1(x(first.at))},${r1(baseline)}${run.map((p) => `L${r1(x(p.at))},${r1(y(p.value!))}`).join("")}L${r1(x(last.at))},${r1(baseline)}Z`;
}

/**
 * The stretches of [t0, t1] where no series has a reading, so the chart can mark them as missing.
 * Covered time is each run from its first to its last reading.
 */
export function missingSpans(runsPerSeries: ChartPoint[][][], t0: number, t1: number): [number, number][] {
  const covered = runsPerSeries.flat().map((r) => [r[0]!.at, r[r.length - 1]!.at] as [number, number]).sort((a, b) => a[0] - b[0]);
  const spans: [number, number][] = [];
  let cursor = t0;
  for (const [a, b] of covered) {
    if (a > cursor) spans.push([cursor, Math.min(a, t1)]);
    cursor = Math.max(cursor, b);
    if (cursor >= t1) break;
  }
  if (cursor < t1) spans.push([cursor, t1]);
  return spans.filter(([a, b]) => b > a);
}

/** The index of the timestamp nearest `at` in an ascending list, or -1 for an empty list. */
export function nearestIndex(times: number[], at: number): number {
  if (!times.length) return -1;
  let lo = 0, hi = times.length - 1;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (times[mid]! < at) lo = mid + 1;
    else hi = mid;
  }
  return lo > 0 && at - times[lo - 1]! <= times[lo]! - at ? lo - 1 : lo;
}

/** Every timestamp any series has, ascending and without repeats. */
export function unionTimes(series: { points: ChartPoint[] }[]): number[] {
  return [...new Set(series.flatMap((s) => s.points.map((p) => p.at)).filter(Number.isFinite))].sort((a, b) => a - b);
}

const short = (n: number) => (Number.isInteger(n) || Math.abs(n) >= 10 ? String(Math.round(n)) : n.toFixed(1));

function binary(n: number): string {
  const sign = n < 0 ? "-" : "";
  const a = Math.abs(n);
  const e = byteExponent(a);
  return `${sign}${short(clean(a / KIB ** e))} ${byteUnits[e]}`;
}

/** A reading in its unit: 42%, 1.5 GB, 12 MB/s. Missing is null, for the caller to word. */
export function formatValue(v: number | null, unit: ChartUnit): string | null {
  if (v === null || !Number.isFinite(v)) return null;
  switch (unit) {
    case "fraction": return `${short(clean(v * 100))}%`;
    case "bytes": return binary(v);
    case "bytes_per_second": return `${binary(v)}/s`;
    default: return short(v);
  }
}

export type SeriesSummary = { latest: number | null; min: number | null; max: number | null; missing: number; total: number };

/** The numbers a text summary needs: the last reading, the range, and how many readings are missing. */
export function summarize(points: ChartPoint[]): SeriesSummary {
  const sorted = [...points].sort((a, b) => a.at - b.at);
  const present = sorted.map((p) => p.value).filter((v): v is number => v !== null && Number.isFinite(v));
  return {
    latest: sorted.length ? sorted[sorted.length - 1]!.value : null,
    min: present.length ? Math.min(...present) : null,
    max: present.length ? Math.max(...present) : null,
    missing: sorted.length - present.length,
    total: sorted.length,
  };
}
