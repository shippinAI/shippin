// Runner sizes and the runners service's plan limits, as staff edit them (RUN-01, docs/runners.md). Kept
// apart from React so it is tested on its own; the types are the generated ones.
import type { RunnerSize, RunnerSizeCreateInput } from "../../../../../lib/api/actions.gen";

/** What the size form edits. */
export type SizeFields = Pick<RunnerSizeCreateInput, "id" | "label" | "cores" | "memory_mib" | "disk_gib" | "image"> & { active: boolean };

export const blankSize: SizeFields = { id: "", label: "", cores: 2, memory_mib: 8192, disk_gib: 50, image: "", active: true };

export const sizeFields = (s: RunnerSize): SizeFields => ({
  id: s.id,
  label: s.label,
  cores: s.cores,
  memory_mib: s.memory_mib,
  disk_gib: s.disk_gib,
  image: s.image,
  active: s.active,
});

const slug = /^[a-z0-9][a-z0-9-]{0,30}$/;

/** What is wrong with a size before it is sent, as the server words it; null when it may go. */
export function sizeProblem(f: SizeFields): string | null {
  if (!slug.test(f.id)) return "A runner size needs a short lowercase id, such as 2x or large; jobs ask for it as shippin-<id>.";
  if (!f.label.trim() || f.label.length > 80) return "Give the size a label of up to 80 characters that says what a customer gets.";
  const whole = (n: number) => Number.isInteger(n);
  if (
    !whole(f.cores) ||
    !whole(f.memory_mib) ||
    !whole(f.disk_gib) ||
    f.cores < 1 ||
    f.cores > 128 ||
    f.memory_mib < 512 ||
    f.memory_mib > 1048576 ||
    f.disk_gib < 8 ||
    f.disk_gib > 4096
  )
    return "A runner needs 1 to 128 cores, 512 MiB to 1 TiB of memory and 8 GiB to 4 TiB of disk.";
  if (f.image.length > 200) return "The image name is at most 200 characters.";
  return null;
}

/** "4 cores · 8 GiB memory · 50 GiB disk". */
export function sizeSummary(s: Pick<RunnerSize, "cores" | "memory_mib" | "disk_gib">): string {
  const mem = s.memory_mib % 1024 === 0 ? `${s.memory_mib / 1024} GiB` : `${s.memory_mib} MiB`;
  return `${s.cores} ${s.cores === 1 ? "core" : "cores"} · ${mem} memory · ${s.disk_gib} GiB disk`;
}

/** The catalog service whose plan cells hold runner limits; the job intake reads svc.runners.* (migration 074). */
export const runnersService = "runners";

/** The runners service's limits on a plan: the sizes it includes, how many jobs run at once (0 runs
 * nothing) and the minutes a month (0 sets no cap). */
export type RunnerLimits = { sizes: string[]; concurrent: number; minutes: number };

const count = (v: unknown) => (typeof v === "number" && Number.isFinite(v) && v >= 0 ? Math.floor(v) : 0);

/** Reads the limits from a cell's open-ended JSON, defensively. */
export function readRunnerLimits(l: Record<string, unknown>): RunnerLimits {
  const sizes = Array.isArray(l.sizes) ? l.sizes.filter((s): s is string => typeof s === "string") : [];
  return { sizes: [...new Set(sizes)], concurrent: count(l.concurrent), minutes: count(l.minutes) };
}

/** Writes the limits back, keeping any other key the cell already holds. */
export const writeRunnerLimits = (l: Record<string, unknown>, r: RunnerLimits): Record<string, unknown> => ({
  ...l,
  sizes: [...r.sizes].sort(),
  concurrent: count(r.concurrent),
  minutes: count(r.minutes),
});

/** The matrix cell's one line, in words: "2x, 4x · 2 at once · 600 min a month". */
export function runnerLimitsBrief(l: Record<string, unknown>): string {
  const r = readRunnerLimits(l);
  return [
    r.sizes.length ? r.sizes.join(", ") : "no sizes",
    r.concurrent ? `${r.concurrent} at once` : "0 at once (runs nothing)",
    r.minutes ? `${r.minutes} min a month` : "no minute cap",
  ].join(" · ");
}
