import { Search, X } from "lucide-react";
import type { ReactNode } from "react";
import { Button as RacButton, Input, SearchField } from "react-aria-components";

type Props = { label: string; value: string; onChange: (v: string) => void; count: number; actions?: ReactNode };

/** Search + count bar above dense tables. */
export function TableToolbar({ label, value, onChange, count, actions }: Props) {
  return (
    <div className="flex items-center gap-3 border-b border-line px-4 py-2.5">
      <SearchField aria-label={`Search ${label}`} value={value} onChange={onChange} className="group relative w-72">
        <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-fg-subtle" />
        <Input
          placeholder={`Search ${label}…`}
          className="h-8 w-full rounded-[var(--radius-control)] border border-line bg-canvas/60 pr-8 pl-8 text-[13px] text-fg outline-none placeholder:text-fg-subtle data-focused:border-primary data-focused:ring-2 data-focused:ring-primary/25 [&::-webkit-search-cancel-button]:hidden"
        />
        <RacButton aria-label="Clear search" className="absolute top-1/2 right-1.5 -translate-y-1/2 rounded p-1 text-fg-subtle group-data-empty:hidden data-hovered:text-fg">
          <X className="size-3" />
        </RacButton>
      </SearchField>
      <span className="text-xs text-fg-subtle tabular-nums">{count} shown</span>
      <div className="ml-auto flex items-center gap-2">{actions}</div>
    </div>
  );
}

/** Case-insensitive match of a query against any of the given fields. */
export const matches = (q: string, ...fields: (string | number | undefined)[]) =>
  !q || fields.some((f) => String(f ?? "").toLowerCase().includes(q.toLowerCase()));
