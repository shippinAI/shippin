import { Check } from "lucide-react";
import { Radio, RadioGroup } from "react-aria-components";
import type { Template } from "../../../../lib/api/types";
import { TemplateIcon } from "./template-icon";

type Props = { templates: Template[]; value?: string; onChange: (id: string) => void };

/** Template cards as an accessible radio group (arrow keys move, Space selects). */
export function TemplateGallery({ templates, value, onChange }: Props) {
  return (
    <RadioGroup aria-label="Template" value={value ?? null} onChange={onChange} className="grid gap-3 sm:grid-cols-2">
      {templates.map((t) => (
        <Radio
          key={t.id}
          value={t.id}
          className="group focus-ring relative flex cursor-default gap-4 rounded-xl border border-line bg-surface p-4.5 shadow-e1 transition-colors data-hovered:border-line-strong data-selected:border-primary data-selected:ring-2 data-selected:ring-primary/25"
        >
          <TemplateIcon icon={t.icon} />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2 text-lg font-semibold text-fg">{t.name}</div>
            <p className="mt-1 text-sm leading-snug text-fg-muted">{t.description}</p>
            <div className="mt-3 flex flex-wrap gap-1.5">
              {t.toolchain.map((x) => (
                <span key={x} className="rounded-md border border-line bg-raised/60 px-1.5 py-0.5 text-2xs text-fg-muted">{x}</span>
              ))}
            </div>
            <div className="mt-3 text-xs text-fg-subtle">
              {t.cpu} vCPU · {t.memory_gib} GiB RAM · {t.disk_gib} GiB disk{t.ports.length ? ` · preview :${t.ports.join(", :")}` : ""}
            </div>
          </div>
          <span className="absolute top-3 right-3 hidden size-5 items-center justify-center rounded-full bg-primary text-primary-fg group-data-selected:flex">
            <Check className="size-3.5" strokeWidth={2.5} />
          </span>
        </Radio>
      ))}
    </RadioGroup>
  );
}
