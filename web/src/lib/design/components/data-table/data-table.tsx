import type { ReactNode } from "react";
import { ChevronDown, ChevronUp, ChevronsUpDown } from "lucide-react";
import { Cell, Column, Row, Table, TableBody, TableHeader } from "react-aria-components";

export type ColumnDef<T> = {
  id: string;
  label: string;
  render: (row: T) => ReactNode;
  width?: string;
  isRowHeader?: boolean;
  sortable?: boolean;
};
export type TableSort = { column: string; direction: "ascending" | "descending" };

type Props<T extends { id: string }> = {
  label: string;
  columns: ColumnDef<T>[];
  rows: T[];
  empty: ReactNode;
  /** Row activation (click / Enter), e.g. open a detail drawer. */
  onRowAction?: (id: string) => void;
  dense?: boolean;
  /** Values the cells read besides the row (React Aria caches rows until one of these changes). */
  dependencies?: readonly unknown[];
  /** The column the rows are sorted by. The caller sorts the rows; the table shows and announces the order. */
  sort?: TableSort;
  onSortChange?: (sort: TableSort) => void;
};

/** Accessible data table (RAC) with keyboard row navigation and optional sortable columns. */
export function DataTable<T extends { id: string }>({
  label,
  columns,
  rows,
  empty,
  onRowAction,
  dense,
  dependencies,
  sort,
  onSortChange,
}: Props<T>) {
  return (
    <Table
      aria-label={label}
      onRowAction={onRowAction ? (k) => onRowAction(String(k)) : undefined}
      sortDescriptor={sort}
      onSortChange={onSortChange ? (d) => onSortChange({ column: String(d.column), direction: d.direction }) : undefined}
      className="w-full border-separate border-spacing-0 text-base"
    >
      <TableHeader columns={columns}>
        {(c) => (
          <Column
            id={c.id}
            isRowHeader={c.isRowHeader}
            allowsSorting={c.sortable}
            style={{ width: c.width }}
            className={`focus-ring border-b border-line bg-surface text-left outline-none ${dense ? "px-4 py-2" : "px-5 py-2.5"} text-xs font-medium tracking-wide text-fg-subtle uppercase data-[allows-sorting]:cursor-pointer data-hovered:text-fg-muted`}
          >
            {({ allowsSorting, sortDirection }) => (
              <span className="inline-flex items-center gap-1">
                {c.label}
                {allowsSorting &&
                  (sortDirection === "ascending" ? (
                    <ChevronUp className="size-3 text-fg" aria-hidden />
                  ) : sortDirection === "descending" ? (
                    <ChevronDown className="size-3 text-fg" aria-hidden />
                  ) : (
                    <ChevronsUpDown className="size-3" aria-hidden />
                  ))}
              </span>
            )}
          </Column>
        )}
      </TableHeader>
      <TableBody items={rows} dependencies={dependencies} renderEmptyState={() => empty}>
        {(row) => (
          <Row
            id={row.id}
            columns={columns}
            dependencies={dependencies}
            className="focus-ring group cursor-default outline-none data-[href]:cursor-pointer data-hovered:bg-raised/60 data-focus-visible:bg-raised"
          >
            {(c) => (
              <Cell className={`border-b border-line text-fg-muted group-last:border-b-0 ${dense ? "px-4 py-2 text-sm" : "px-5 py-3"}`}>
                {c.render(row)}
              </Cell>
            )}
          </Row>
        )}
      </TableBody>
    </Table>
  );
}
