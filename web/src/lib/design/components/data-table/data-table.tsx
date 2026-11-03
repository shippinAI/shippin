import type { ReactNode } from "react";
import { Cell, Column, Row, Table, TableBody, TableHeader } from "react-aria-components";

export type ColumnDef<T> = { id: string; label: string; render: (row: T) => ReactNode; width?: string; isRowHeader?: boolean };

type Props<T extends { id: string }> = {
  label: string;
  columns: ColumnDef<T>[];
  rows: T[];
  empty: ReactNode;
  /** Row activation (click / Enter), e.g. open a detail drawer. */
  onRowAction?: (id: string) => void;
  dense?: boolean;
};

/** Accessible data table (RAC) with keyboard row navigation. */
export function DataTable<T extends { id: string }>({ label, columns, rows, empty, onRowAction, dense }: Props<T>) {
  return (
    <Table aria-label={label} onRowAction={onRowAction ? (k) => onRowAction(String(k)) : undefined} className="w-full border-separate border-spacing-0 text-sm">
      <TableHeader columns={columns}>
        {(c) => (
          <Column
            id={c.id}
            isRowHeader={c.isRowHeader}
            style={{ width: c.width }}
            className={`border-b border-line bg-surface text-left ${dense ? "px-4 py-2" : "px-5 py-2.5"} text-[11.5px] font-medium tracking-wide text-fg-subtle uppercase`}
          >
            {c.label}
          </Column>
        )}
      </TableHeader>
      <TableBody items={rows} renderEmptyState={() => empty}>
        {(row) => (
          <Row
            id={row.id}
            columns={columns}
            className="focus-ring group cursor-default outline-none data-[href]:cursor-pointer data-hovered:bg-raised/60 data-focus-visible:bg-raised"
          >
            {(c) => (
              <Cell className={`border-b border-line text-fg-muted group-last:border-b-0 ${dense ? "px-4 py-2 text-[13px]" : "px-5 py-3"}`}>{c.render(row)}</Cell>
            )}
          </Row>
        )}
      </TableBody>
    </Table>
  );
}
