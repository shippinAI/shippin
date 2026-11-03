import { useState } from "react";
import { ChevronLeft, ChevronRight, Plus, Trash2 } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { Select } from "../../../lib/design/components/select/select";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { TextField } from "../../../lib/design/components/field/text-field";
import type { DbColumn, DbTable } from "../../../lib/api/database-types";
import { cellText, containsFilter, rowCountLabel } from "../model";
import { useAddColumn, useDbBrowse, useDeleteRow, useSaveRow } from "../../../lib/hooks/use-database";
import { columnTypes } from "../../../lib/api/database-types";

const ops = [
  { id: "eq", label: "is" },
  { id: "neq", label: "is not" },
  { id: "contains", label: "contains" },
  { id: "gt", label: "greater than" },
  { id: "lt", label: "less than" },
  { id: "isnull", label: "is empty" },
  { id: "notnull", label: "is filled" },
];

export function TableEditor({ tenantId, table, readOnly }: { tenantId: string; table: DbTable; readOnly: boolean }) {
  const [offset, setOffset] = useState(0);
  const [sort, setSort] = useState("");
  const [dir, setDir] = useState<"asc" | "desc">("asc");
  const [column, setColumn] = useState("");
  const [op, setOp] = useState("eq");
  const [value, setValue] = useState("");
  const [applied, setApplied] = useState<{ column?: string; op?: string; value?: string }>({});
  const [edit, setEdit] = useState<{ key: string; column: string; draft: string } | null>(null);
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [pendingDelete, setPendingDelete] = useState<Record<string, unknown> | null>(null);
  const [colOpen, setColOpen] = useState(false);
  const [newCol, setNewCol] = useState({ name: "", type: "text", nullable: true, default: "" });
  const limit = 50;
  const page = useDbBrowse(tenantId, { schema: table.schema, table: table.name, limit, offset, sort, dir, ...applied });
  const save = useSaveRow(tenantId, table.name, readOnly);
  const remove = useDeleteRow(tenantId, table.name, readOnly);
  const addCol = useAddColumn(tenantId, table.name, readOnly);
  const columns = page.data?.columns ?? [];
  const pkCols = columns.filter((c) => c.primary_key);
  const rows = page.data?.rows ?? [];
  const total = page.data?.total ?? 0;

  const pkOf = (row: Record<string, unknown>) => {
    const pk: Record<string, unknown> = {};
    for (const c of pkCols) pk[c.name] = row[c.name];
    return pk;
  };
  const rowKey = (row: Record<string, unknown>, i: number) => (pkCols.length ? JSON.stringify(pkOf(row)) : String(i));

  const commit = (row: Record<string, unknown>, col: DbColumn, raw: string) => {
    if (readOnly) return;
    const next = raw === "" && col.nullable ? null : parseCell(col, raw);
    save.mutate({ pk: pkOf(row), values: { [col.name]: next }, insert: false });
    setEdit(null);
  };

  const toggleSort = (name: string) => {
    if (sort === name) setDir(dir === "asc" ? "desc" : "asc");
    else {
      setSort(name);
      setDir("asc");
    }
    setOffset(0);
  };

  return (
    <Card className="overflow-hidden">
      <CardHeader
        title={<span className="font-mono">{table.schema}.{table.name}</span>}
        description={page.data ? `${rowCountLabel({ row_count: total, estimate: table.estimate })}${page.data.platform ? " · created for platform services" : ""}` : "Loading rows"}
        actions={
          <>
            {table.platform && <Badge tone="warning">Platform</Badge>}
            <Button size="sm" variant="secondary" tooltip="Add a column with a type from the list" isDisabled={readOnly || table.schema !== "public"} onPress={() => setColOpen((v) => !v)}>
              <Plus className="size-3.5" /> Column
            </Button>
            <Button size="sm" variant="primary" tooltip={pkCols.length ? "Add a row" : "This table needs a primary key before rows can be edited"} isDisabled={readOnly || pkCols.length === 0} onPress={() => { setAdding(true); setDraft({}); }}>
              <Plus className="size-3.5" /> Row
            </Button>
          </>
        }
      />
      {table.platform && (
        <p className="border-b border-line px-5 py-2.5 text-[12.5px] text-fg-muted">
          This table holds references for secrets, identity, or DNS. Values of secrets are not stored here. Deleting a row asks you to confirm.
        </p>
      )}
      <div className="flex flex-wrap items-end gap-2 border-b border-line px-4 py-3">
        <div className="w-40">
          <Select label="Column" options={[{ id: "", label: "Any column" }, ...columns.map((c) => ({ id: c.name, label: c.name }))]} value={column} onChange={setColumn} />
        </div>
        <div className="w-36">
          <Select label="Match" options={ops} value={op} onChange={setOp} />
        </div>
        {op !== "isnull" && op !== "notnull" && (
          <div className="min-w-40 flex-1">
            <TextField label="Value" value={value} onChange={setValue} placeholder="optional" />
          </div>
        )}
        <Button size="sm" variant="secondary" tooltip="Show rows that match" onPress={() => {
          if (!column) {
            setApplied({});
            setOffset(0);
            return;
          }
          if (op === "contains") {
            const f = containsFilter(value);
            setApplied({ column, op: f.op, value: f.value });
          } else setApplied({ column, op, value });
          setOffset(0);
        }}>
          Filter
        </Button>
      </div>
      {colOpen && (
        <form
          className="grid gap-3 border-b border-line px-4 py-3 sm:grid-cols-[1fr_180px_auto]"
          onSubmit={(e) => {
            e.preventDefault();
            addCol.mutate(newCol, { onSuccess: () => { setColOpen(false); setNewCol({ name: "", type: "text", nullable: true, default: "" }); } });
          }}
        >
          <TextField label="New column" value={newCol.name} onChange={(v) => setNewCol({ ...newCol, name: v })} />
          <Select label="Type" options={columnTypes} value={newCol.type} onChange={(v) => setNewCol({ ...newCol, type: v })} />
          <div className="flex items-end">
            <Button type="submit" variant="primary" size="sm" tooltip="Add this column" isDisabled={readOnly || !newCol.name}>Add</Button>
          </div>
        </form>
      )}
      {page.isPending && <div className="p-4"><Skeleton className="h-40" /></div>}
      {page.isError && <p className="px-5 py-4 text-[13px] text-fg-muted">{(page.error as Error).message}</p>}
      {page.data && (
        <div className="overflow-x-auto">
          <table className="w-full border-separate border-spacing-0 text-sm" aria-label={`${table.name} rows`}>
            <thead>
              <tr>
                {columns.map((c) => (
                  <th key={c.name} className="border-b border-line bg-surface px-3 py-2 text-left text-[11.5px] font-medium tracking-wide text-fg-subtle uppercase">
                    <button type="button" className="focus-ring inline-flex items-center gap-1 rounded" onClick={() => toggleSort(c.name)}>
                      {c.name}
                      {c.primary_key && <span className="text-primary normal-case">pk</span>}
                      {sort === c.name && <span aria-hidden>{dir === "asc" ? "↑" : "↓"}</span>}
                    </button>
                  </th>
                ))}
                <th className="w-12 border-b border-line bg-surface" />
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 && (
                <tr>
                  <td colSpan={columns.length + 1} className="px-4 py-10 text-center text-[13px] text-fg-muted">No rows on this page. Add one, or loosen the filter.</td>
                </tr>
              )}
              {rows.map((row, i) => {
                const key = rowKey(row, i);
                return (
                  <tr key={key} className="hover:bg-raised/50">
                    {columns.map((c) => {
                      const active = edit?.key === key && edit.column === c.name;
                      return (
                        <td key={c.name} className="border-b border-line px-3 py-1.5 font-mono text-[12.5px] text-fg">
                          {active ? (
                            <input
                              autoFocus
                              aria-label={`Edit ${c.name}`}
                              className="focus-ring w-full min-w-32 rounded border border-primary bg-canvas px-2 py-1 text-[12.5px] outline-none"
                              value={edit.draft}
                              onChange={(e) => setEdit({ ...edit, draft: e.target.value })}
                              onBlur={() => commit(row, c, edit.draft)}
                              onKeyDown={(e) => {
                                if (e.key === "Enter") commit(row, c, edit.draft);
                                if (e.key === "Escape") setEdit(null);
                              }}
                            />
                          ) : (
                            <button
                              type="button"
                              className="focus-ring block w-full truncate rounded px-1 py-1 text-left"
                              title={pkCols.length ? "Click to edit" : "Add a primary key before editing"}
                              onClick={() => pkCols.length && !readOnly && setEdit({ key, column: c.name, draft: cellText(row[c.name]) })}
                            >
                              {row[c.name] == null ? <span className="text-fg-subtle">null</span> : cellText(row[c.name])}
                            </button>
                          )}
                        </td>
                      );
                    })}
                    <td className="border-b border-line px-1">
                      <IconButton icon={Trash2} label="Delete row" isDisabled={readOnly || pkCols.length === 0} onPress={() => setPendingDelete(pkOf(row))} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {adding && (
        <form
          className="grid gap-3 border-t border-line px-4 py-4 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            const values: Record<string, unknown> = {};
            for (const c of columns) {
              if (draft[c.name] == null || draft[c.name] === "") continue;
              values[c.name] = parseCell(c, draft[c.name]);
            }
            save.mutate({ values, insert: true }, { onSuccess: () => { setAdding(false); setDraft({}); } });
          }}
        >
          {columns.map((c) => (
            <TextField key={c.name} label={c.name} description={c.primary_key ? "Primary key" : c.type} value={draft[c.name] ?? ""} onChange={(v) => setDraft({ ...draft, [c.name]: v })} />
          ))}
          <div className="flex items-end gap-2 sm:col-span-2">
            <Button type="submit" variant="primary" size="sm" tooltip="Insert this row" isDisabled={readOnly}>Save row</Button>
            <Button type="button" variant="ghost" size="sm" tooltip="Discard the new row" onPress={() => setAdding(false)}>Cancel</Button>
          </div>
        </form>
      )}
      <div className="flex items-center justify-between px-4 py-3 text-[12.5px] text-fg-subtle">
        <span>{total === 0 ? "No rows" : `${offset + 1}–${Math.min(offset + limit, total)} of ${total.toLocaleString()}`}</span>
        <span className="flex gap-1">
          <IconButton icon={ChevronLeft} label="Previous page" isDisabled={offset === 0} onPress={() => setOffset(Math.max(0, offset - limit))} />
          <IconButton icon={ChevronRight} label="Next page" isDisabled={offset + limit >= total} onPress={() => setOffset(offset + limit)} />
        </span>
      </div>
      {pkCols.length === 0 && <p className="border-t border-line px-5 py-3 text-[12.5px] text-fg-muted">This table has no primary key, so rows are read-only. Create tables with an id column when you want to edit them here.</p>}
      <ConfirmDialog
        isOpen={!!pendingDelete}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        title="Delete this row?"
        body="The row is removed from your database. This asks the server to confirm, and it is written to the SQL audit log."
        confirmLabel="Delete row"
        onConfirm={() => pendingDelete && remove.mutate(pendingDelete)}
      />
    </Card>
  );
}

function parseCell(col: DbColumn, raw: string): unknown {
  if (col.type === "boolean") return raw === "true" || raw === "t" || raw === "yes";
  if (col.type === "integer" || col.type === "bigint" || col.type === "smallint") {
    const n = Number(raw);
    return Number.isFinite(n) ? n : raw;
  }
  if (col.type === "numeric") {
    const n = Number(raw);
    return Number.isFinite(n) ? n : raw;
  }
  if (col.type === "jsonb" || col.type === "json") {
    try { return JSON.parse(raw); } catch { return raw; }
  }
  return raw;
}
