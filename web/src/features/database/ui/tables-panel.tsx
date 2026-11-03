import { useState } from "react";
import { ListTodo, Newspaper, Plus, Users } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { Checkbox } from "../../../lib/design/components/checkbox/checkbox";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { Select } from "../../../lib/design/components/select/select";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { TextField } from "../../../lib/design/components/field/text-field";
import { columnTypes, type DbTable } from "../../../lib/api/database-types";
import { rowCountLabel, userTables } from "../model";
import { useApplyTemplate, useCreateTable, useDbTables, useDbTemplates } from "../../../lib/hooks/use-database";
import { TableEditor } from "./table-editor";

const icons: Record<string, LucideIcon> = { todo: ListTodo, crm: Users, blog: Newspaper };

type DraftCol = { name: string; type: string; nullable: boolean; primary_key: boolean; default: string };

const idColumn = (): DraftCol => ({ name: "id", type: "uuid", nullable: false, primary_key: true, default: "gen_random_uuid()" });

export function TablesPanel({ tenantId, readOnly }: { tenantId: string; readOnly: boolean }) {
  const tables = useDbTables(tenantId);
  const templates = useDbTemplates(tenantId);
  const apply = useApplyTemplate(tenantId, readOnly);
  const create = useCreateTable(tenantId, readOnly);
  const [selected, setSelected] = useState<DbTable | null>(null);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [cols, setCols] = useState<DraftCol[]>([idColumn(), { name: "title", type: "text", nullable: false, primary_key: false, default: "" }]);
  const items = tables.data?.tables ?? [];
  const mine = userTables(items);
  const current = selected && items.some((t) => t.schema === selected.schema && t.name === selected.name) ? selected : null;

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-[280px_1fr]">
      <Card className="overflow-hidden">
        <CardHeader
          title="Tables"
          description="Row counts are exact when the count finishes in time."
          actions={
            <Button size="sm" variant="secondary" tooltip="Create a table with a type for each column" onPress={() => setOpen(true)} isDisabled={!tenantId}>
              <Plus className="size-3.5" /> New
            </Button>
          }
        />
        {tables.isPending && <div className="space-y-2 p-4">{Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-12" />)}</div>}
        {tables.isError && <p className="px-5 py-4 text-[13px] text-fg-muted">{(tables.error as Error).message}</p>}
        <div className="border-b border-line px-4 py-3">
          <div className="mb-2 text-[11.5px] font-medium tracking-wide text-fg-subtle uppercase">Start from a template</div>
          <div className="flex flex-wrap gap-1.5">
            {(templates.data?.templates ?? []).map((tpl) => (
              <Button
                key={tpl.id}
                size="sm"
                variant="ghost"
                tooltip={readOnly ? "Turn off read-only to create tables" : tpl.hint}
                isDisabled={readOnly || apply.isPending}
                onPress={() => apply.mutate(tpl.id, { onSuccess: () => setSelected({ schema: "public", name: tpl.tables[0], row_count: 0, estimate: 0, platform: false }) })}
              >
                {tpl.title}
              </Button>
            ))}
          </div>
        </div>
        <ul className="max-h-[32rem] overflow-y-auto" aria-label="Tables">
          {items.map((t) => {
            const on = current?.schema === t.schema && current?.name === t.name;
            return (
              <li key={`${t.schema}.${t.name}`}>
                <button
                  type="button"
                  onClick={() => setSelected(t)}
                  className={`focus-ring flex w-full items-center justify-between gap-3 border-b border-line px-4 py-3 text-left last:border-b-0 ${on ? "bg-primary/10" : "hover:bg-raised/70"}`}
                >
                  <span className="min-w-0">
                    <span className="block truncate font-mono text-[13px] text-fg">{t.name}</span>
                    <span className="text-[11.5px] text-fg-subtle">{t.schema === "public" ? rowCountLabel(t) : t.schema}</span>
                  </span>
                  {t.platform && <Badge>Platform</Badge>}
                </button>
              </li>
            );
          })}
        </ul>
      </Card>

      <div className="min-w-0">
        {current ? (
          <TableEditor tenantId={tenantId} table={current} readOnly={readOnly} />
        ) : (
          <Card>
            <EmptyState
              title={mine.length === 0 ? "Start with a shape you already know" : "Pick a table"}
              description={mine.length === 0
                ? "These templates create ordinary tables in your database. Vaultix secret storage stays separate and does not show up here."
                : "Choose a table on the left to browse and edit rows."}
            />
            {mine.length === 0 && (
              <div className="grid gap-3 px-5 pb-5 sm:grid-cols-3">
                {(templates.data?.templates ?? []).map((tpl) => {
                  const Icon = icons[tpl.id] ?? ListTodo;
                  return (
                    <div key={tpl.id} className="rounded-xl border border-line bg-canvas/40 p-4">
                      <Icon className="mb-3 size-5 text-hue-data" aria-hidden />
                      <div className="text-sm font-medium text-fg">{tpl.title}</div>
                      <p className="mt-1 text-[12.5px] leading-relaxed text-fg-muted">{tpl.description}</p>
                      <p className="mt-2 text-[12px] text-fg-subtle">{tpl.hint}</p>
                      <Button
                        className="mt-3"
                        size="sm"
                        variant="primary"
                        tooltip={readOnly ? "Turn off read-only to create tables" : `Create ${tpl.tables.join(", ")}`}
                        isDisabled={readOnly || apply.isPending}
                        onPress={() => apply.mutate(tpl.id, { onSuccess: () => setSelected({ schema: "public", name: tpl.tables[0], row_count: 0, estimate: 0, platform: false }) })}
                      >
                        Use {tpl.title}
                      </Button>
                    </div>
                  );
                })}
              </div>
            )}
          </Card>
        )}
      </div>

      <ModalDialog isOpen={open} onOpenChange={setOpen} title="New table" description="Names stay simple: letters, numbers, and underscores. Pick a type instead of writing SQL.">
        <form
          className="space-y-4 px-6 py-5"
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate({ name, columns: cols }, { onSuccess: () => { setOpen(false); setSelected({ schema: "public", name, row_count: 0, estimate: 0, platform: false }); } });
          }}
        >
          <TextField label="Table name" value={name} onChange={setName} placeholder="todos" description="This lands in the public schema." />
          <div className="space-y-3">
            {cols.map((c, i) => (
              <div key={i} className="grid gap-2 rounded-lg border border-line p-3 sm:grid-cols-[1fr_160px]">
                <TextField label="Column" value={c.name} onChange={(v) => setCols(cols.map((x, j) => j === i ? { ...x, name: v } : x))} />
                <Select label="Type" options={columnTypes} value={c.type} onChange={(v) => setCols(cols.map((x, j) => j === i ? { ...x, type: v, default: "" } : x))} />
                <div className="flex flex-wrap gap-2 sm:col-span-2">
                  <Checkbox isSelected={c.primary_key} onChange={(on) => setCols(cols.map((x, j) => j === i ? { ...x, primary_key: on, nullable: on ? false : x.nullable } : { ...x, primary_key: on ? false : x.primary_key }))}>
                    Primary key
                  </Checkbox>
                  <Checkbox isSelected={c.nullable} isDisabled={c.primary_key} onChange={(on) => setCols(cols.map((x, j) => j === i ? { ...x, nullable: on } : x))}>
                    Can be empty
                  </Checkbox>
                  <DefaultField type={c.type} value={c.default} onChange={(v) => setCols(cols.map((x, j) => j === i ? { ...x, default: v } : x))} />
                </div>
              </div>
            ))}
          </div>
          <div className="flex justify-between gap-2">
            <Button type="button" variant="ghost" tooltip="Add another column" onPress={() => setCols([...cols, { name: "", type: "text", nullable: true, primary_key: false, default: "" }])}>
              Add column
            </Button>
            <Button type="submit" variant="primary" tooltip="Create this table" isDisabled={readOnly || create.isPending || !name}>
              Create table
            </Button>
          </div>
        </form>
      </ModalDialog>
    </div>
  );
}

function DefaultField({ type, value, onChange }: { type: string; value: string; onChange: (v: string) => void }) {
  if (type === "uuid") return <Select label="Default" options={[{ id: "", label: "None" }, { id: "gen_random_uuid()", label: "Random UUID" }]} value={value} onChange={onChange} />;
  if (type === "timestamptz" || type === "date") return <Select label="Default" options={[{ id: "", label: "None" }, { id: "now()", label: "Now" }]} value={value} onChange={onChange} />;
  if (type === "boolean") return <Select label="Default" options={[{ id: "", label: "None" }, { id: "false", label: "No" }, { id: "true", label: "Yes" }]} value={value} onChange={onChange} />;
  if (type === "integer" || type === "bigint" || type === "numeric") {
    return <TextField label="Default" value={value} onChange={onChange} placeholder="Optional number" />;
  }
  return null;
}
