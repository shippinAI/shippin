import assert from "node:assert/strict";
import {
  areaPath,
  formatValue,
  linearScale,
  linePath,
  missingSpans,
  nearestIndex,
  niceStep,
  sampleStep,
  segments,
  summarize,
  timeTickKind,
  timeTicks,
  unionTimes,
  valueDomain,
  valueTicks,
  type ChartPoint,
} from "./chart-model";
import { meterLevel, usedFraction } from "../meter/meter-model";

const GB = 1024 ** 3,
  MIN = 60_000,
  HOUR = 60 * MIN,
  DAY = 24 * HOUR;
const pts = (...vs: (number | null)[]): ChartPoint[] => vs.map((value, i) => ({ at: i * MIN, value }));

// Scales
const s = linearScale([0, 10], [100, 200]);
assert.equal(s(0), 100);
assert.equal(s(5), 150);
assert.equal(s.invert(175), 7.5);
const inverted = linearScale([0, 1], [50, 10]);
assert.equal(inverted(1), 10, "a value axis runs bottom to top");
assert.equal(linearScale([3, 3], [0, 10])(3), 0, "an empty domain does not divide by zero");

// Ticks
assert.equal(niceStep(0.23), 0.25);
assert.equal(niceStep(3), 5);
assert.equal(niceStep(0.7), 1);
assert.equal(niceStep(140), 200);
assert.equal(niceStep(0), 1);
assert.deepEqual(valueTicks(0, 1, "fraction"), { ticks: [0, 0.25, 0.5, 0.75, 1], top: 1 }, "a fraction ticks in quarters to 100 %");
assert.deepEqual(valueTicks(0, 0.37, "fraction").ticks, [0, 0.1, 0.2, 0.3, 0.4], "and reaches past the largest reading");
assert.deepEqual(
  valueTicks(0, 7.3 * GB, "bytes").ticks,
  [0, 2, 4, 6, 8].map((n) => n * GB),
  "bytes tick in whole binary units",
);
assert.deepEqual(
  valueTicks(0, 300 * 1024 * 1024, "bytes_per_second").ticks.map((v) => formatValue(v, "bytes_per_second")),
  ["0 B/s", "100 MB/s", "200 MB/s", "300 MB/s"],
);
assert.deepEqual(valueTicks(0, 0, "count"), { ticks: [0, 0.25, 0.5, 0.75, 1], top: 1 }, "an all-zero series still has an axis");
assert.equal(valueTicks(0, 13, "count").top, 15);

assert.deepEqual(valueDomain([0.2, null, 0.4], "fraction"), [0, 1], "a fraction axis shows the whole");
assert.deepEqual(valueDomain([0.2, 1.4], "fraction"), [0, 1.4], "unless a reading is above it");
assert.deepEqual(valueDomain([2 * GB, null], "bytes", 8 * GB), [0, 8 * GB], "a ceiling such as total memory");
assert.deepEqual(valueDomain([null, null], "bytes"), [0, 1], "no readings, still a domain");
assert.deepEqual(valueDomain([-3, 2], "count"), [-3, 2]);

const tt = timeTicks(0, 6 * HOUR, 6);
assert.equal(tt.step, HOUR);
assert.deepEqual(
  tt.ticks,
  [0, 1, 2, 3, 4, 5, 6]
    .map((h) => h * HOUR)
    .filter((t) => t <= 6 * HOUR)
    .slice(0, 7),
);
assert.equal(timeTicks(7 * MIN, 67 * MIN, 4).ticks[0], 15 * MIN, "ticks land on round times");
assert.equal(timeTicks(0, 7 * DAY, 8).step, DAY);
assert.equal(timeTicks(0, 7 * DAY, 8, 2 * HOUR).ticks[0], DAY - 2 * HOUR, "a day tick is local midnight");
assert.equal(timeTickKind(HOUR), "time");
assert.equal(timeTickKind(DAY), "date");

// Gaps stay gaps
assert.deepEqual(
  segments(pts(1, 2, null, 4, 5)).map((r) => r.map((p) => p.value)),
  [
    [1, 2],
    [4, 5],
  ],
  "a null breaks the line",
);
assert.deepEqual(segments(pts(null, null)).length, 0, "nothing to draw is nothing, not a flat zero");
assert.deepEqual(
  segments(pts(1, null, 3, null, 5)).map((r) => r.length),
  [1, 1, 1],
  "isolated readings stay single points",
);
assert.deepEqual(
  segments(pts(0, 0, 0)).map((r) => r.length),
  [3],
  "zero is a reading, not a gap",
);
const absent: ChartPoint[] = [0, 1, 2, 6, 7].map((m) => ({ at: m * MIN, value: m }));
assert.equal(sampleStep(absent), MIN);
assert.deepEqual(
  segments(absent).map((r) => r.map((p) => p.value)),
  [
    [0, 1, 2],
    [6, 7],
  ],
  "rows the sampler never wrote break the line too",
);
assert.deepEqual(segments(absent, 10 * MIN).length, 1, "unless the caller allows that long a gap");
assert.deepEqual(
  segments([
    { at: 2 * MIN, value: 2 },
    { at: 0, value: 0 },
    { at: MIN, value: 1 },
  ]).map((r) => r.map((p) => p.at)),
  [[0, MIN, 2 * MIN]],
  "readings are put in time order",
);

const x = (t: number) => t / MIN,
  y = (v: number) => 10 - v;
assert.equal(linePath(pts(1, 2, 3), x, y), "M0,9L1,8L2,7");
assert.equal(areaPath(pts(1, 2), x, y, 10), "M0,10L0,9L1,8L1,10Z");
assert.equal(areaPath(pts(1), x, y, 10), "", "one reading has no area");
const runs = segments(pts(1, 2, null, null, 5, 6));
assert.ok(!linePath(runs[0]!, x, y).includes("L4"), "the line never reaches across the gap");

assert.deepEqual(
  missingSpans([segments(pts(1, 2, null, null, 5, 6))], 0, 5 * MIN),
  [[MIN, 4 * MIN]],
  "the gap is marked from the last reading to the next",
);
assert.deepEqual(
  missingSpans([segments(pts(1, 2))], -MIN, 3 * MIN),
  [
    [-MIN, 0],
    [MIN, 3 * MIN],
  ],
  "time before the first and after the last reading is missing",
);
assert.deepEqual(
  missingSpans([segments(pts(1, 2, null)), segments(pts(null, 2, 3))], 0, 2 * MIN),
  [],
  "a stretch one series covers is not missing",
);
assert.deepEqual(
  missingSpans([segments(pts(1, null, 3)), segments(pts(null, 2, null))], 0, 2 * MIN),
  [
    [0, MIN],
    [MIN, 2 * MIN],
  ],
  "isolated readings cover only their own instant",
);
assert.deepEqual(missingSpans([[]], 0, 10), [[0, 10]], "no readings at all: the whole range is missing");

assert.equal(nearestIndex([0, 10, 20], 4), 0);
assert.equal(nearestIndex([0, 10, 20], 6), 1);
assert.equal(nearestIndex([0, 10, 20], 99), 2);
assert.equal(nearestIndex([0, 10, 20], -5), 0);
assert.equal(nearestIndex([], 3), -1);
assert.deepEqual(
  unionTimes([
    { points: pts(1, 2) },
    {
      points: [
        { at: MIN, value: 1 },
        { at: 5 * MIN, value: null },
      ],
    },
  ]),
  [0, MIN, 5 * MIN],
);

// Units
assert.equal(formatValue(0.42, "fraction"), "42%");
assert.equal(formatValue(0.025, "fraction"), "2.5%");
assert.equal(formatValue(0, "fraction"), "0%");
assert.equal(formatValue(1.5 * GB, "bytes"), "1.5 GB");
assert.equal(formatValue(512 * 1024 * 1024, "bytes"), "512 MB");
assert.equal(formatValue(0, "bytes"), "0 B", "zero bytes is a reading");
assert.equal(formatValue(900, "bytes"), "900 B");
assert.equal(formatValue(12 * 1024 * 1024, "bytes_per_second"), "12 MB/s");
assert.equal(formatValue(3, "count"), "3");
assert.equal(formatValue(null, "bytes"), null, "missing is for the caller to word");
assert.equal(formatValue(Number.NaN, "fraction"), null);

assert.deepEqual(summarize(pts(0.1, null, 0.5, 0.3)), { latest: 0.3, min: 0.1, max: 0.5, missing: 1, total: 4 });
assert.deepEqual(
  summarize(pts(0.1, null)),
  { latest: null, min: 0.1, max: 0.1, missing: 1, total: 2 },
  "a missing last reading is not the previous one",
);
assert.deepEqual(summarize([]), { latest: null, min: null, max: null, missing: 0, total: 0 });

// Meter
assert.equal(usedFraction(2, 8), 0.25);
assert.equal(usedFraction(0, 8), 0, "nothing used is a reading");
assert.equal(usedFraction(9, 8), 1, "over the total is full, not more");
assert.equal(usedFraction(undefined, 8), null, "unknown use is not empty");
assert.equal(usedFraction(2, 0), null, "no total, no meter");
assert.equal(usedFraction(2, null), null);
assert.equal(meterLevel(0.5), "normal");
assert.equal(meterLevel(0.8), "warning");
assert.equal(meterLevel(0.95), "danger");
assert.equal(meterLevel(null), "normal");

console.log("chart model ok");
