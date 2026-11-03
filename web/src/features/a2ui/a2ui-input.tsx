import { Label, Radio, RadioGroup } from "react-aria-components";
import { Checkbox } from "../../lib/design/components/checkbox/checkbox";
import { TextField } from "../../lib/design/components/field/text-field";
import { getPath, resolve } from "../../lib/a2ui/a2ui-model";
import type { A2UIComponent } from "../../lib/a2ui/a2ui-types";
import { useSurface } from "./a2ui-surface";

/** Two-way bound inputs: edits write the local data model only (A2UI v0.9 read/write contract). */
export function A2UIInput({ c, scope }: { c: A2UIComponent; scope: string }) {
  const { surface, onData } = useSurface();
  const d = surface.data;
  if (c.component !== "TextField" && c.component !== "CheckBox" && c.component !== "ChoicePicker") return null;
  const abs = c.value.path.startsWith("/") ? c.value.path : `${scope}/${c.value.path}`;
  const cur = getPath(d, abs);
  const label = String(resolve(c.label, d, scope) ?? "");
  if (c.component === "TextField") {
    return (
      <TextField label={label} value={String(cur ?? "")} onChange={(v) => onData(abs, v)} multiline={c.variant === "longText"}
        type={c.variant === "obscured" ? "password" : c.variant === "number" ? "number" : "text"} />
    );
  }
  if (c.component === "CheckBox") return <Checkbox isSelected={!!cur} onChange={(v) => onData(abs, v)}>{label}</Checkbox>;
  const sel = Array.isArray(cur) ? String(cur[0] ?? "") : String(cur ?? "");
  return (
    <RadioGroup value={sel} onChange={(v) => onData(abs, [v])} className="flex flex-col gap-1.5">
      {label && <Label className="text-[13px] font-medium text-fg">{label}</Label>}
      {c.options.map((o) => (
        <Radio key={o.value} value={o.value} className="group flex items-center gap-2 text-[13px] text-fg-muted outline-none data-selected:text-fg">
          <span className="size-3.5 rounded-full border border-line-strong group-data-selected:border-4 group-data-selected:border-primary group-data-focus-visible:ring-2 group-data-focus-visible:ring-primary/40" />
          {o.label}
        </Radio>
      ))}
    </RadioGroup>
  );
}
