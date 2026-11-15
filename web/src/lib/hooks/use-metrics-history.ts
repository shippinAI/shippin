import { useQuery } from "@tanstack/react-query";
import { callAction } from "../api/actions";

// Sampled history for one host or one guest (OBS-01), from the metrics range action
// (shippin.infrastructure.metric.list, GET /v1/operator/infrastructure/metrics, staff infra.read).
// This file is the only place that knows the action; screens use the types below, which mirror its output
// (MetricRange and friends in actions.gen.ts), narrowed where the generated types are plain strings.

export type MetricName =
  | "cpu" | "load1" | "memory_used_bytes" | "memory_total_bytes" | "disk_used_bytes" | "disk_total_bytes"
  | "net_in_bytes_per_second" | "net_out_bytes_per_second";
/** cpu is a fraction of the whole (0 to 1); load is the one-minute load average. */
export type MetricUnit = "fraction" | "load" | "bytes" | "bytes_per_second";
/** One step: the average and peak of the samples in it. A step with no sample is null, never zero. */
export type MetricBucket = { avg: number; max: number; samples: number };
/** Buckets line up with MetricRange.times. */
export type MetricSeries = { metric: MetricName; unit: MetricUnit; buckets: (MetricBucket | null)[] };
export type MetricSamplerState = {
  state: "sampling" | "standby" | "idle" | "off" | "starting";
  reason?: string;
  interval_seconds: number;
  last_sampled_at: string | null;
};
export type MetricRange = {
  server_id: string;
  /** Empty for the host itself. */
  guest_id: string;
  from: string;
  /** Exclusive. */
  to: string;
  step_seconds: number;
  /** The start of each step. */
  times: string[];
  series: MetricSeries[];
  sampler: MetricSamplerState;
};

export type MetricsTarget = { serverId: string; guest?: string };
export const historyRanges = ["1h", "24h", "7d"] as const;
export type HistoryRange = (typeof historyRanges)[number];

/** Each range's span and step, kept well under the action's 1500 steps. */
const rangeSpec: Record<HistoryRange, { seconds: number; step: number }> = {
  "1h": { seconds: 3600, step: 60 },
  "24h": { seconds: 86_400, step: 300 },
  "7d": { seconds: 604_800, step: 3600 },
};

export const historyMetrics: MetricName[] = [
  "cpu", "memory_used_bytes", "memory_total_bytes", "disk_used_bytes", "disk_total_bytes", "net_in_bytes_per_second", "net_out_bytes_per_second",
];

/** The query for one range ending now: from rounded down to a step, to one step past now so the current step shows. */
export function rangeQuery(target: MetricsTarget, range: HistoryRange, now = Date.now()): URLSearchParams {
  const { seconds, step } = rangeSpec[range];
  const stepMs = step * 1000;
  const to = Math.ceil(now / stepMs) * stepMs;
  return new URLSearchParams({
    server_id: target.serverId, guest_id: target.guest ?? "", metrics: historyMetrics.join(","),
    from: new Date(to - seconds * 1000).toISOString(), to: new Date(to).toISOString(), step: String(step),
  });
}

const fetchRange = async (target: MetricsTarget, range: HistoryRange): Promise<MetricRange> =>
  (await callAction("shippin.infrastructure.metric.list", Object.fromEntries(rangeQuery(target, range)))) as MetricRange;

export type MetricsHistoryResult = { data: MetricRange | undefined; isPending: boolean; error: Error | null };

/** History for one host, or one of its guests, over a range. No data, and not pending, when nothing records history yet. */
export function useMetricsHistory(target: MetricsTarget, range: HistoryRange): MetricsHistoryResult {
  const enabled = !!target.serverId;
  const q = useQuery({
    queryKey: ["metrics", "history", target.serverId, target.guest ?? "", range],
    queryFn: () => fetchRange(target, range),
    enabled,
    refetchInterval: 60_000,
    // A new range keeps the old charts until it arrives; a different host or guest never shows another's.
    placeholderData: (prev, prevQuery) => (prevQuery?.queryKey[2] === target.serverId && prevQuery.queryKey[3] === (target.guest ?? "") ? prev : undefined),
  });
  return { data: enabled ? q.data : undefined, isPending: enabled && q.isPending, error: q.error };
}
