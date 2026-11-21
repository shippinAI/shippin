import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent, type RefObject } from "react";
import { cn } from "../cn/cn";
import {
  areaPath,
  formatValue,
  linearScale,
  linePath,
  missingSpans,
  nearestIndex,
  segments,
  timeTickKind,
  timeTicks,
  unionTimes,
  valueDomain,
  valueTicks,
  type ChartPoint,
  type ChartUnit,
} from "./chart-model";

/** Series colours are the module hues, in a fixed order, so the same position always has the same colour. */
export type SeriesHue = "data" | "identity" | "network" | "secrets" | "edge" | "ai" | "billing" | "workspaces";
const hueOrder: SeriesHue[] = ["data", "identity", "network", "secrets", "edge", "ai", "billing", "workspaces"];
const strokes: Record<SeriesHue, string> = {
  data: "stroke-hue-data",
  identity: "stroke-hue-identity",
  network: "stroke-hue-network",
  secrets: "stroke-hue-secrets",
  edge: "stroke-hue-edge",
  ai: "stroke-hue-ai",
  billing: "stroke-hue-billing",
  workspaces: "stroke-hue-workspaces",
};
const dots: Record<SeriesHue, string> = {
  data: "fill-hue-data",
  identity: "fill-hue-identity",
  network: "fill-hue-network",
  secrets: "fill-hue-secrets",
  edge: "fill-hue-edge",
  ai: "fill-hue-ai",
  billing: "fill-hue-billing",
  workspaces: "fill-hue-workspaces",
};
const areas: Record<SeriesHue, string> = {
  data: "fill-hue-data/15",
  identity: "fill-hue-identity/15",
  network: "fill-hue-network/15",
  secrets: "fill-hue-secrets/15",
  edge: "fill-hue-edge/15",
  ai: "fill-hue-ai/15",
  billing: "fill-hue-billing/15",
  workspaces: "fill-hue-workspaces/15",
};
const swatches: Record<SeriesHue, string> = {
  data: "bg-hue-data",
  identity: "bg-hue-identity",
  network: "bg-hue-network",
  secrets: "bg-hue-secrets",
  edge: "bg-hue-edge",
  ai: "bg-hue-ai",
  billing: "bg-hue-billing",
  workspaces: "bg-hue-workspaces",
};

export type ChartSeries = { id: string; label: string; points: ChartPoint[]; hue?: SeriesHue };

type Props = {
  /** The chart's accessible name and the caption of its table of numbers. */
  label: string;
  /** One or two sentences a screen reader hears instead of the picture. */
  summary: string;
  unit: ChartUnit;
  series: ChartSeries[];
  /** The time range drawn. Time inside it with no reading is marked as missing. Defaults to the readings' own range. */
  from?: number;
  to?: number;
  /** A value the axis reaches even if no reading does, such as a host's total memory. */
  ceiling?: number;
  /** Fill under the line. Only ever drawn for a single series. */
  area?: boolean;
  /** Wording, from the caller's configured text. */
  text: { noData: string; time: string };
  /** Height, as a size class. */
  className?: string;
};

const margin = { top: 8, right: 12, bottom: 22, left: 56 };
const fallback = { w: 640, h: 192 };

function useBoxSize(ref: RefObject<HTMLDivElement | null>) {
  const [size, setSize] = useState(fallback);
  useEffect(() => {
    const el = ref.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(([e]) => {
      const r = e?.contentRect;
      if (r && r.width > 0 && r.height > 0) setSize({ w: r.width, h: r.height });
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [ref]);
  return size;
}

const timeFmt = new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" });
const dateFmt = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short" });
const fullFmt = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short", hour: "numeric", minute: "2-digit" });

/**
 * A time series in SVG with no charting library. A null reading, or a missing stretch of readings, breaks the line
 * and is shaded as missing; nothing is joined across it. Hover or arrow keys read the values at a time.
 */
export function TimeSeriesChart({ label, summary, unit, series, from, to, ceiling, area = true, text, className }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const { w, h } = useBoxSize(box);
  const summaryId = useId();
  const [cursor, setCursor] = useState<number | null>(null);
  const [focused, setFocused] = useState(false);

  const model = useMemo(() => {
    const coloured = series.map((s, i) => ({ ...s, hue: s.hue ?? hueOrder[i % hueOrder.length]! }));
    const times = unionTimes(coloured);
    const t0 = from ?? times[0] ?? 0,
      t1Raw = to ?? times[times.length - 1] ?? 0;
    const t1 = t1Raw > t0 ? t1Raw : t0 + 60_000;
    const values = coloured.flatMap((s) => s.points.map((p) => p.value));
    const [lo, hi] = valueDomain(values, unit, ceiling);
    const vt = valueTicks(lo, hi, unit, h < 160 ? 3 : 4);
    const runs = coloured.map((s) => segments(s.points));
    const byTime = coloured.map((s) => new Map(s.points.map((p) => [p.at, p.value])));
    return {
      coloured,
      times,
      t0,
      t1,
      lo: Math.min(lo, vt.ticks[0] ?? lo),
      top: vt.top,
      vTicks: vt.ticks,
      runs,
      byTime,
      gaps: missingSpans(runs, t0, t1),
    };
  }, [series, from, to, ceiling, unit, h]);

  const plotW = Math.max(1, w - margin.left - margin.right),
    plotH = Math.max(1, h - margin.top - margin.bottom);
  const x = linearScale([model.t0, model.t1], [margin.left, margin.left + plotW]);
  const y = linearScale([model.lo, model.top], [margin.top + plotH, margin.top]);
  const offset = -new Date(model.t0).getTimezoneOffset() * 60_000;
  const tt = timeTicks(model.t0, model.t1, Math.max(2, Math.floor(plotW / 90)), offset);
  const tickFmt = timeTickKind(tt.step) === "date" ? dateFmt : timeFmt;
  const fillArea = area && model.coloured.length === 1;
  const at = cursor !== null ? model.times[cursor] : undefined;
  const empty = model.times.length === 0;

  const move = (i: number) => setCursor(Math.max(0, Math.min(model.times.length - 1, i)));
  const onKey = (e: KeyboardEvent) => {
    if (empty) return;
    const i = cursor ?? model.times.length - 1;
    const step = { ArrowLeft: -1, ArrowRight: 1, PageUp: -10, PageDown: 10 }[e.key];
    if (step !== undefined) move(cursor === null ? i : i + step);
    else if (e.key === "Home") move(0);
    else if (e.key === "End") move(model.times.length - 1);
    else if (e.key === "Escape") setCursor(null);
    else return;
    e.preventDefault();
  };
  const onPointer = (e: PointerEvent) => {
    if (empty) return;
    const left = e.currentTarget.getBoundingClientRect().left;
    setCursor(nearestIndex(model.times, x.invert(e.clientX - left)));
  };

  const readout = model.coloured.map((s, i) => {
    const v = at !== undefined ? (model.byTime[i]!.get(at) ?? null) : ([...s.points].sort((a, b) => a.at - b.at).at(-1)?.value ?? null);
    return { id: s.id, label: s.label, hue: s.hue, value: formatValue(v, unit) ?? text.noData };
  });

  return (
    <figure className="min-w-0">
      <div className="mb-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs" aria-live={focused ? "polite" : "off"}>
        <span className="text-fg-subtle tabular-nums">{at !== undefined ? fullFmt.format(at) : " "}</span>
        {readout.map((r) => (
          <span key={r.id} className="inline-flex items-center gap-1.5">
            <span className={cn("inline-block size-2 rounded-sm", swatches[r.hue])} aria-hidden />
            <span className="text-fg-muted">{r.label}</span>
            <span className="font-medium text-fg tabular-nums">{r.value}</span>
          </span>
        ))}
      </div>
      <div
        ref={box}
        role="group"
        tabIndex={0}
        aria-label={label}
        aria-describedby={summaryId}
        onKeyDown={onKey}
        onPointerMove={onPointer}
        onPointerLeave={() => setCursor(null)}
        onFocus={() => setFocused(true)}
        onBlur={() => {
          setFocused(false);
          setCursor(null);
        }}
        className={cn("focus-ring relative touch-pan-y rounded-md outline-none", className ?? "h-48")}
      >
        <svg width={w} height={h} className="block" aria-hidden>
          {model.gaps.map(([a, b]) => (
            <rect key={a} x={x(a)} y={margin.top} width={Math.max(1, x(b) - x(a))} height={plotH} className="fill-raised" />
          ))}
          {model.vTicks.map((v) => (
            <g key={v}>
              <line x1={margin.left} x2={margin.left + plotW} y1={y(v)} y2={y(v)} className="stroke-line" />
              <text x={margin.left - 8} y={y(v)} dy="0.32em" textAnchor="end" className="fill-fg-subtle text-2xs tabular-nums">
                {formatValue(v, unit)}
              </text>
            </g>
          ))}
          {tt.ticks.map((t) => (
            // A tick near either edge is anchored inward so its label is not cut off.
            <text
              key={t}
              x={x(t)}
              y={h - 6}
              textAnchor={x(t) - margin.left < 28 ? "start" : margin.left + plotW - x(t) < 28 ? "end" : "middle"}
              className="fill-fg-subtle text-2xs tabular-nums"
            >
              {tickFmt.format(t)}
            </text>
          ))}
          {model.coloured.map((s, i) => (
            <g key={s.id}>
              {fillArea &&
                model.runs[i]!.map((run) => (
                  <path key={`a${run[0]!.at}`} d={areaPath(run, x, y, margin.top + plotH)} className={areas[s.hue]} />
                ))}
              {model.runs[i]!.map((run) =>
                run.length === 1 ? (
                  <circle key={run[0]!.at} cx={x(run[0]!.at)} cy={y(run[0]!.value!)} r={2} className={dots[s.hue]} />
                ) : (
                  <path
                    key={run[0]!.at}
                    d={linePath(run, x, y)}
                    fill="none"
                    strokeWidth={1.5}
                    strokeLinejoin="round"
                    strokeLinecap="round"
                    className={strokes[s.hue]}
                  />
                ),
              )}
            </g>
          ))}
          {at !== undefined && (
            <g>
              <line x1={x(at)} x2={x(at)} y1={margin.top} y2={margin.top + plotH} className="stroke-line-strong" />
              {model.coloured.map((s, i) => {
                const v = model.byTime[i]!.get(at);
                return v === null || v === undefined ? null : (
                  <circle key={s.id} cx={x(at)} cy={y(v)} r={3} className={cn(dots[s.hue], "stroke-surface")} strokeWidth={1.5} />
                );
              })}
            </g>
          )}
          {empty && (
            <text x={margin.left + plotW / 2} y={margin.top + plotH / 2} textAnchor="middle" className="fill-fg-subtle text-xs">
              {text.noData}
            </text>
          )}
        </svg>
      </div>
      <figcaption id={summaryId} className="sr-only">
        {summary}
      </figcaption>
      <table className="sr-only">
        <caption>{label}</caption>
        <thead>
          <tr>
            <th scope="col">{text.time}</th>
            {model.coloured.map((s) => (
              <th key={s.id} scope="col">
                {s.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {model.times.map((t) => (
            <tr key={t}>
              <th scope="row">{fullFmt.format(t)}</th>
              {model.coloured.map((s, i) => (
                <td key={s.id}>{formatValue(model.byTime[i]!.get(t) ?? null, unit) ?? text.noData}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  );
}
