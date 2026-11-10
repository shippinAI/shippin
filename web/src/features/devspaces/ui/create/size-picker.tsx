import { Cpu } from "lucide-react";
import { Radio, RadioGroup } from "react-aria-components";
import type { SizePreset } from "../../../../lib/api/types";

type Props = { sizes: SizePreset[]; value: string; onChange: (id: string) => void; recommended?: string };

export function SizePicker({ sizes, value, onChange, recommended }: Props) {
  return (
    <RadioGroup aria-label="Size" value={value} onChange={onChange} className="grid gap-3 sm:grid-cols-3">
      {sizes.map((z) => (
        <Radio
          key={z.id}
          value={z.id}
          className="focus-ring cursor-default rounded-xl border border-line bg-surface p-4 transition-colors data-hovered:border-line-strong data-selected:border-primary data-selected:ring-2 data-selected:ring-primary/25"
        >
          <div className="flex items-center justify-between">
            <span className="text-base font-semibold text-fg">{z.label}</span>
            {z.id === recommended && <span className="rounded-full bg-primary/12 px-2 py-0.5 text-2xs font-medium text-primary">Template default</span>}
          </div>
          <div className="mt-3 flex items-center gap-2 text-2xl font-semibold tracking-tight tabular-nums">
            <Cpu className="size-4 text-fg-subtle" /> {z.cpu} <span className="text-base font-normal text-fg-muted">vCPU</span>
          </div>
          <div className="mt-1 text-sm text-fg-muted">{z.memory_gib} GiB RAM · {z.disk_gib} GiB disk</div>
        </Radio>
      ))}
    </RadioGroup>
  );
}
