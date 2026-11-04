import { ChevronDown } from "lucide-react";
import { Button, Label, ListBox, ListBoxItem, Popover, Select as RacSelect, SelectValue, type Key } from "react-aria-components";

type Option = { id: string; label: string };
type Props = { label: string; options: Option[]; value: string; onChange: (v: string) => void; isDisabled?: boolean };

/** Accessible select (RAC) styled like our inputs. */
export function Select({ label, options, value, onChange, isDisabled = false }: Props) {
  return (
    <RacSelect selectedKey={value} onSelectionChange={(k: Key | null) => k !== null && onChange(String(k))} isDisabled={isDisabled} className="flex flex-col gap-1.5">
      <Label className="text-[13px] font-medium text-fg">{label}</Label>
      <Button className="focus-ring flex h-9 cursor-default items-center justify-between rounded-[var(--radius-control)] border border-line bg-canvas/60 px-3 text-sm text-fg data-hovered:border-line-strong">
        <SelectValue />
        <ChevronDown className="size-4 text-fg-subtle" />
      </Button>
      <Popover className="glass min-w-[var(--trigger-width)] rounded-lg border border-line-strong p-1 shadow-[var(--shadow-e3)]">
        <ListBox className="outline-none">
          {options.map((o) => (
            <ListBoxItem key={o.id} id={o.id} className="cursor-default rounded-md px-2.5 py-1.5 text-sm text-fg-muted outline-none data-focused:bg-raised data-focused:text-fg data-selected:text-primary">
              {o.label}
            </ListBoxItem>
          ))}
        </ListBox>
      </Popover>
    </RacSelect>
  );
}
