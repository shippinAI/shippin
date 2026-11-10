import { Check, Minus } from "lucide-react";
import type { ReactNode } from "react";
import { Checkbox as RacCheckbox, type CheckboxProps } from "react-aria-components";

type Props = Omit<CheckboxProps, "children" | "className"> & { children: ReactNode; description?: ReactNode };

/** Checkbox (RAC) with an optional second line. */
export function Checkbox({ children, description, ...rest }: Props) {
  return (
    <RacCheckbox {...rest} className="group flex cursor-default items-start gap-2.5 rounded-md px-2 py-1.5 text-sm text-fg-muted outline-none data-hovered:bg-raised/60 data-disabled:opacity-50">
      {({ isSelected, isIndeterminate }) => (
        <>
          <span className="focus-ring mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-sm border border-line-control bg-canvas/60 transition-colors group-data-focus-visible:ring-2 group-data-focus-visible:ring-primary/40 group-data-selected:border-primary group-data-selected:bg-primary">
            {isIndeterminate ? <Minus className="size-3 text-primary-fg" /> : isSelected ? <Check className="size-3 text-primary-fg" strokeWidth={3} /> : null}
          </span>
          <span className="min-w-0">
            <span className="block text-fg">{children}</span>
            {description && <span className="block text-xs text-fg-subtle">{description}</span>}
          </span>
        </>
      )}
    </RacCheckbox>
  );
}
