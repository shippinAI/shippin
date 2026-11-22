import assert from "node:assert/strict";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { createJiti } from "jiti";
import { rangeQuery, type MetricRange } from "../../../hooks/use-metrics-history";
import { textFor } from "../../../ui-config/model";
import type { TextKey } from "../../../ui-config/text-keys";

// The components are TSX; load them with JSX turned on, the way Vite builds them.
const tsx = createJiti(import.meta.url, { jsx: { runtime: "automatic" } });
const { TimeSeriesChart } = await tsx.import<typeof import("./time-series-chart")>("./time-series-chart.tsx");
const { HistoryCharts, samplerText } = await tsx.import<typeof import("../../../../features/infrastructure/ui/metrics-history")>(
  "../../../../features/infrastructure/ui/metrics-history.tsx",
);

// Fixture: ten minutes of one-minute samples with readings missing at minutes 3 and 4.
const start = Date.UTC(2026, 10, 15, 12, 0);
const MIN = 60_000;
const values = [0.12, 0.18, 0.22, null, null, 0.4, 0.35, 0.3, 0.28, 0.25];
const points = values.map((value, i) => ({ at: start + i * MIN, value }));
const text = { noData: "No data", time: "Time" };

const html = renderToStaticMarkup(
  createElement(TimeSeriesChart, {
    label: "CPU",
    summary: "CPU average: latest 25%, between 12% and 40%. 2 of 10 samples are missing.",
    unit: "fraction",
    series: [{ id: "cpu", label: "Average", points }],
    from: start,
    to: start + 9 * MIN,
    text,
  }),
);

const lines = [...html.matchAll(/<path d="(M[^"]+)" fill="none"/g)].map((m) => m[1]!);
assert.equal(lines.length, 2, "the gap splits the line in two");
assert.equal(lines[0]!.split("L").length, 3, "the first run has the three readings before the gap");
assert.equal(lines[1]!.split("L").length, 5, "the second run has the five after it");
const areas = [...html.matchAll(/<path d="(M[^"]+Z)"/g)];
assert.equal(areas.length, 2, "a single series is filled, run by run, never across the gap");
const shaded = [...html.matchAll(/<rect x="([\d.]+)" y="[\d.]+" width="([\d.]+)"[^>]*class="fill-raised"/g)];
assert.equal(shaded.length, 1, "the missing stretch is shaded");
assert.ok(Number(shaded[0]![2]) > 0);
assert.ok(html.includes('role="group"') && html.includes('tabindex="0"'), "the plot takes keyboard focus");
assert.ok(html.includes("2 of 10 samples are missing"), "the summary is in the page for screen readers");
assert.equal((html.match(/<td>No data<\/td>/g) ?? []).length, 2, "the table of numbers says missing, not zero");
assert.ok(html.includes("<td>40%</td>"));
assert.ok(!html.includes("<td>0%</td>"), "no missing reading became zero");
for (const tick of ["0%", "25%", "50%"]) assert.ok(html.includes(`>${tick}</text>`), `value axis tick ${tick}`);
assert.ok(!/#[0-9a-f]{3,8}\b|oklch\(|rgb\(/i.test(html), "colours come from token classes");

const two = renderToStaticMarkup(
  createElement(TimeSeriesChart, {
    label: "Network",
    summary: "s",
    unit: "bytes_per_second",
    series: [
      {
        id: "in",
        label: "In",
        points: [
          { at: start, value: 1024 },
          { at: start + MIN, value: 2048 },
        ],
      },
      {
        id: "out",
        label: "Out",
        points: [
          { at: start, value: 512 },
          { at: start + MIN, value: null },
        ],
      },
    ],
    text,
  }),
);
assert.equal([...two.matchAll(/<path d="M[^"]+Z"/g)].length, 0, "several series are lines only");
assert.ok(two.includes("stroke-hue-data") && two.includes("fill-hue-identity"), "each series gets its own hue; a lone reading is a dot");
assert.ok(two.includes("KB/s"));

const empty = renderToStaticMarkup(
  createElement(TimeSeriesChart, { label: "Disk", summary: "none", unit: "bytes", series: [{ id: "d", label: "Used", points: [] }], text }),
);
assert.ok(empty.includes(">No data</text>"), "no readings says so");

// The host history section, drawn from a fixture the metrics range action could return.
const t = (key: TextKey, vars?: Record<string, string | number>) => textFor(undefined, key, vars);
const iso = (i: number) => new Date(start + i * MIN).toISOString();
const bucket = (v: number | null, k = 1) => (v === null ? null : { avg: v * k, max: v * k + 0.1, samples: 3 });
const history: MetricRange = {
  server_id: "s",
  guest_id: "",
  from: iso(0),
  to: iso(10),
  step_seconds: 60,
  times: values.map((_, i) => iso(i)),
  sampler: { state: "sampling", interval_seconds: 60, last_sampled_at: iso(9) },
  series: [
    { metric: "cpu", unit: "fraction", buckets: values.map((v) => bucket(v)) },
    { metric: "memory_used_bytes", unit: "bytes", buckets: values.map((v) => bucket(v, 8 * 1024 ** 3)) },
    { metric: "disk_used_bytes", unit: "bytes", buckets: values.map(() => null) },
    { metric: "net_in_bytes_per_second", unit: "bytes_per_second", buckets: [bucket(10), ...values.slice(1).map(() => null)] },
  ],
};
const section = renderToStaticMarkup(createElement(HistoryCharts, { history, t }));
assert.ok(
  section.includes('data-testid="history-cpu"') &&
    section.includes('data-testid="history-memory"') &&
    section.includes('data-testid="history-network"'),
);
assert.ok(!section.includes('data-testid="history-disk"'), "a metric with no steps has no chart");
assert.ok(section.includes("2 of 10 samples are missing."), "the summary counts the gap");
assert.ok(section.includes("Peak") && section.includes("Average"));

assert.equal(
  samplerText(t, { state: "off", reason: "ClickHouse is not connected", interval_seconds: 60, last_sampled_at: null }),
  "The metrics sampler is off: ClickHouse is not connected",
);

const q = rangeQuery({ serverId: "11111111-2222-3333-4444-555555555555" }, "24h", Date.UTC(2026, 10, 15, 12, 2, 30));
assert.equal(q.get("guest_id"), "", "the host itself");
assert.equal(q.get("step"), "300");
assert.equal(q.get("to"), "2026-11-15T12:05:00.000Z", "to is exclusive, so the current step is included");
assert.equal(q.get("from"), "2026-11-14T12:05:00.000Z");
assert.equal(q.get("metrics")!.split(",").length <= 8, true, "the action takes at most eight metrics");
assert.equal(rangeQuery({ serverId: "s", guest: "101" }, "7d").get("guest_id"), "101");

console.log("time series chart ok");
