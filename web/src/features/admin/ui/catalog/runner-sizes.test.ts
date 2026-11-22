import assert from "node:assert/strict";
import { blankSize, readRunnerLimits, runnerLimitsBrief, sizeFields, sizeProblem, sizeSummary, writeRunnerLimits } from "./runner-sizes";

// The form refuses what the server's check refuses, in the same words.
const ok = { ...blankSize, id: "2x", label: "2 cores, 8 GiB" };
assert.equal(sizeProblem(ok), null);
assert.match(sizeProblem({ ...ok, id: "" }) ?? "", /short lowercase id/);
assert.match(sizeProblem({ ...ok, id: "Big" }) ?? "", /short lowercase id/);
assert.match(sizeProblem({ ...ok, id: "-x" }) ?? "", /short lowercase id/);
assert.match(sizeProblem({ ...ok, id: "a".repeat(32) }) ?? "", /short lowercase id/);
assert.equal(sizeProblem({ ...ok, id: "a".repeat(31) }), null);
assert.match(sizeProblem({ ...ok, label: " " }) ?? "", /label of up to 80/);
assert.match(sizeProblem({ ...ok, label: "x".repeat(81) }) ?? "", /label of up to 80/);
assert.match(sizeProblem({ ...ok, cores: 0 }) ?? "", /1 to 128 cores/);
assert.match(sizeProblem({ ...ok, cores: 1.5 }) ?? "", /1 to 128 cores/);
assert.match(sizeProblem({ ...ok, memory_mib: 256 }) ?? "", /512 MiB/);
assert.match(sizeProblem({ ...ok, disk_gib: 4 }) ?? "", /8 GiB/);
assert.match(sizeProblem({ ...ok, image: "i".repeat(201) }) ?? "", /200 characters/);
// An empty image is allowed: the size exists, but no job can start on it until one is set.
assert.equal(sizeProblem({ ...ok, image: "" }), null);

assert.equal(sizeSummary({ cores: 4, memory_mib: 8192, disk_gib: 50 }), "4 cores · 8 GiB memory · 50 GiB disk");
assert.equal(sizeSummary({ cores: 1, memory_mib: 1536, disk_gib: 8 }), "1 core · 1536 MiB memory · 8 GiB disk");
assert.deepEqual(
  sizeFields({ id: "2x", label: "L", cores: 2, memory_mib: 4096, disk_gib: 20, image: "img", active: false, updated_at: "t" }),
  { id: "2x", label: "L", cores: 2, memory_mib: 4096, disk_gib: 20, image: "img", active: false },
);

// Plan limits: read defensively, written back with other keys kept.
assert.deepEqual(readRunnerLimits({}), { sizes: [], concurrent: 0, minutes: 0 });
assert.deepEqual(readRunnerLimits({ sizes: ["4x", 3, "2x", "4x"], concurrent: 2.7, minutes: -5 }), {
  sizes: ["4x", "2x"],
  concurrent: 2,
  minutes: 0,
});
assert.deepEqual(readRunnerLimits({ sizes: "2x", concurrent: "3" }), { sizes: [], concurrent: 0, minutes: 0 });
assert.deepEqual(writeRunnerLimits({ other: true, sizes: ["x"] }, { sizes: ["4x", "2x"], concurrent: 3, minutes: 600 }), {
  other: true,
  sizes: ["2x", "4x"],
  concurrent: 3,
  minutes: 600,
});
assert.equal(runnerLimitsBrief({ sizes: ["2x", "4x"], concurrent: 2, minutes: 600 }), "2x, 4x · 2 at once · 600 min a month");
assert.equal(runnerLimitsBrief({}), "no sizes · 0 at once (runs nothing) · no minute cap");

console.log("runner sizes model ok");
