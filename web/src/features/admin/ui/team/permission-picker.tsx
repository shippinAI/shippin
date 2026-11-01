import { CheckboxGroup, Label } from "react-aria-components";
import { Checkbox } from "../../../../lib/design/components/checkbox/checkbox";
import type { Perm } from "../../../../lib/api/team-types";

type Props = { catalog: Perm[]; value: string[]; onChange: (v: string[]) => void; isDisabled?: boolean };

/** The permission catalog grouped by panel section, as checkboxes. */
export function PermissionPicker({ catalog, value, onChange, isDisabled }: Props) {
  const sections = [...new Set(catalog.map((p) => p.section))];
  return (
    <CheckboxGroup value={value} onChange={onChange} isDisabled={isDisabled} aria-label="Permissions" className="grid grid-cols-1 gap-3">
      {sections.map((s) => {
        const ps = catalog.filter((p) => p.section === s);
        const n = ps.filter((p) => value.includes(p.key)).length;
        return (
          <div key={s} className="rounded-lg border border-line bg-canvas/30 p-2">
            <Label className="flex items-center justify-between px-2 pb-1 text-[12px] font-semibold tracking-wide text-fg-subtle uppercase">
              {s} <span className="font-normal tabular-nums normal-case">{n}/{ps.length}</span>
            </Label>
            {ps.map((p) => (
              <Checkbox key={p.key} value={p.key}
                description={<><span className="font-mono">{p.key}</span>{p.high_risk && <span className="ml-1.5 text-warning">high risk</span>}{p.reserved && <span className="ml-1.5">reserved</span>}</>}>
                {p.label}
              </Checkbox>
            ))}
          </div>
        );
      })}
    </CheckboxGroup>
  );
}
