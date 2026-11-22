import { Checkbox } from "../../../../lib/design/components/checkbox/checkbox";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { useRunnerSizes } from "../../../../lib/hooks/use-catalog";
import { readRunnerLimits, sizeSummary, writeRunnerLimits } from "./runner-sizes";

type Props = { limits: Record<string, unknown> | null; onChange: (next: Record<string, unknown>) => void };

const whole = (v: string) => Math.max(0, Math.floor(Number(v) || 0));

/** The Build runners cell's limits in words: which sizes the plan includes, how many jobs run at once and
 * the minutes a month. It edits the same limits object the JSON field below shows. */
export function RunnerLimitsFields({ limits, onChange }: Props) {
  const sizes = useRunnerSizes();
  if (!limits) return <p className="text-sm text-fg-muted">Fix the limits JSON below to edit them here.</p>;
  const r = readRunnerLimits(limits);
  const put = (next: Partial<typeof r>) => onChange(writeRunnerLimits(limits, { ...r, ...next }));
  const known = sizes.data?.items ?? [];
  const unknown = r.sizes.filter((id) => !known.some((s) => s.id === id));
  return (
    <fieldset className="space-y-3 rounded-lg border border-line p-4">
      <legend className="px-1 text-sm font-medium text-fg">Build runner limits</legend>
      <div role="group" aria-label="Sizes this plan includes" className="space-y-0.5">
        <div className="text-sm font-medium text-fg">Sizes this plan includes</div>
        {sizes.isPending && <p className="text-sm text-fg-subtle">Loading runner sizes…</p>}
        {sizes.isError && (
          <p role="alert" className="text-sm text-danger">
            {sizes.error.message}
          </p>
        )}
        {sizes.isSuccess && known.length === 0 && (
          <p className="text-sm text-fg-muted">No runner sizes yet. Add them under Runner sizes first.</p>
        )}
        {known.map((s) => (
          <Checkbox
            key={s.id}
            isSelected={r.sizes.includes(s.id)}
            description={`${sizeSummary(s)}${s.active ? "" : " · off"}`}
            onChange={(on) => put({ sizes: on ? [...r.sizes, s.id] : r.sizes.filter((x) => x !== s.id) })}
          >
            shippin-{s.id} <span className="text-fg-subtle">({s.label})</span>
          </Checkbox>
        ))}
        {unknown.map((id) => (
          <Checkbox
            key={id}
            isSelected
            description="No such runner size any more; jobs asking for it are refused"
            onChange={() => put({ sizes: r.sizes.filter((x) => x !== id) })}
          >
            shippin-{id}
          </Checkbox>
        ))}
      </div>
      <div className="grid grid-cols-2 gap-4">
        <TextField
          label="Jobs at once"
          type="number"
          min={0}
          value={String(r.concurrent)}
          onChange={(v) => put({ concurrent: whole(v) })}
          description="0 runs nothing"
        />
        <TextField
          label="Minutes a month"
          type="number"
          min={0}
          value={String(r.minutes)}
          onChange={(v) => put({ minutes: whole(v) })}
          description="0 sets no monthly cap"
        />
      </div>
    </fieldset>
  );
}
