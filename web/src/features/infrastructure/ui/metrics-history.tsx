import { useState } from "react";
import { ChartLine } from "lucide-react";
import { ApiError } from "../../../lib/api/client";
import { formatValue, summarize } from "../../../lib/design/components/chart/chart-model";
import { TimeSeriesChart } from "../../../lib/design/components/chart/time-series-chart";
import { cn } from "../../../lib/design/components/cn/cn";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { Select } from "../../../lib/design/components/select/select";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { historyRanges, useMetricsHistory, type HistoryRange, type MetricRange, type MetricSamplerState } from "../../../lib/hooks/use-metrics-history";
import type { TextKey } from "../../../lib/ui-config/text-keys";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { historyCharts, type HistoryChart } from "../model";

type Text = ReturnType<typeof useText>;

const chartName = (t: Text, id: HistoryChart["id"]) => t(`infra.history.chart.${id}` as TextKey);
const seriesName = (t: Text, id: string) => t(`infra.history.series.${id}` as TextKey);

/** One sentence per series, for a screen reader: the latest value, the range, and how much is missing. */
function chartSummary(t: Text, chart: HistoryChart): string {
  return chart.series.map((s) => {
    const name = `${chartName(t, chart.id)} ${seriesName(t, s.id).toLowerCase()}`;
    const sum = summarize(s.points);
    if (sum.min === null) return t("infra.history.summaryNone", { series: name });
    const f = (v: number | null) => formatValue(v, chart.unit) ?? t("chart.noData");
    const line = t("infra.history.summary", { series: name, latest: f(sum.latest), min: f(sum.min), max: f(sum.max) });
    return sum.missing ? `${line} ${t("infra.history.summaryMissing", { missing: sum.missing, total: sum.total })}` : line;
  }).join(" ");
}

/** What the sampler says about itself, such as "The metrics sampler is off: ClickHouse is not connected". */
export function samplerText(t: Text, s: MetricSamplerState): string {
  const state = t(`infra.history.sampler.${s.state}` as TextKey, { interval: s.interval_seconds });
  const parts = [s.reason ? `${state}: ${s.reason}` : state];
  if (s.last_sampled_at) parts.push(t("infra.history.sampler.last", { time: new Date(s.last_sampled_at).toLocaleString() }));
  return parts.join(" · ");
}

/** The charts for one range. Exported so a test can draw it from fixture data. */
export function HistoryCharts({ history, t }: { history: MetricRange; t: Text }) {
  const from = Date.parse(history.from), to = Date.parse(history.to);
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      {historyCharts(history).map((chart) => (
        <section key={chart.id} aria-label={chartName(t, chart.id)} data-testid={`history-${chart.id}`}>
          <h4 className="mb-2 text-sm font-medium text-fg">{chartName(t, chart.id)}</h4>
          <TimeSeriesChart
            label={chartName(t, chart.id)}
            summary={chartSummary(t, chart)}
            unit={chart.unit}
            ceiling={chart.ceiling}
            from={Number.isFinite(from) ? from : undefined}
            to={Number.isFinite(to) ? to : undefined}
            series={chart.series.map((s) => ({ id: s.id, label: seriesName(t, s.id), points: s.points }))}
            text={{ noData: t("chart.noData"), time: t("chart.time") }}
            className="h-44"
          />
        </section>
      ))}
    </div>
  );
}

/** Sampled history for a host, or for one of its guests. It shows an empty state until something records history. */
export function MetricsHistory({ serverId, guest, className }: { serverId: string; guest?: string; className?: string }) {
  const t = useText();
  const [range, setRange] = useState<HistoryRange>("24h");
  const q = useMetricsHistory({ serverId, guest }, range);
  const hasCharts = historyCharts(q.data).length > 0;
  // ClickHouse not connected is the sampler being off, not a fault on this page.
  const off = q.error instanceof ApiError && q.error.status === 503;
  const sampler = q.data?.sampler;
  const emptyBody = off ? `${t("infra.history.sampler.off")}: ${q.error!.message}`
    : sampler && sampler.state !== "sampling" ? samplerText(t, sampler)
    : q.data ? t("infra.history.noSamples")
    : t("infra.history.empty.body");
  return (
    <section className={cn("rounded-xl border border-line bg-surface shadow-e1", className)} data-testid="metrics-history" aria-label={t("infra.history.title")}>
      <div className="flex flex-wrap items-end justify-between gap-4 border-b border-line px-5 py-4">
        <div className="min-w-0">
          <h3 className="text-base font-semibold text-fg">{t("infra.history.title")}</h3>
          <p className="mt-0.5 text-sm text-fg-muted">{t("infra.history.description")}</p>
          {sampler && hasCharts && <p className="mt-1 text-xs text-fg-subtle" data-testid="sampler-state">{samplerText(t, sampler)}</p>}
        </div>
        {(q.data !== undefined || q.isPending) && (
          <div className="w-44">
            <Select label={t("infra.history.range")} value={range} onChange={(v) => setRange(v as HistoryRange)}
              options={historyRanges.map((r) => ({ id: r, label: t(`infra.history.range.${r}` as TextKey) }))} />
          </div>
        )}
      </div>
      <div className="p-5">
        {q.isPending ? (
          <div className="grid gap-6 lg:grid-cols-2">{Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-52" />)}</div>
        ) : q.error && !off ? (
          <p role="alert" className="text-base text-danger">{t("infra.history.error", { error: q.error.message })}</p>
        ) : q.data && hasCharts ? (
          <HistoryCharts history={q.data} t={t} />
        ) : (
          <EmptyState illustration={<ChartLine className="size-10 text-fg-subtle" />} title={t("infra.history.empty.title")} description={emptyBody} />
        )}
      </div>
    </section>
  );
}
