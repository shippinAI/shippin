import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  DbAsk, DbAudit, DbBackup, DbBackupList, DbInfo, DbPage, DbQueryResult, DbSnippet, DbTable, DbTemplate,
} from "../api/database-types";
import { toast } from "../design/components/toast/toast-queue";

export class DbError extends Error {
  status: number;
  confirmationRequired: boolean;
  kind: string;
  constructor(status: number, message: string, extra?: { confirmation_required?: boolean; kind?: string }) {
    super(message);
    this.status = status;
    this.confirmationRequired = extra?.confirmation_required === true;
    this.kind = extra?.kind ?? "";
  }
}

async function dbSend<T>(method: "POST" | "PUT" | "PATCH" | "DELETE", path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = res.status === 204 ? {} : await res.json().catch(() => ({}));
  if (!res.ok) {
    const problem = data as { message?: string; confirmation_required?: boolean; kind?: string };
    throw new DbError(res.status, problem.message ?? res.statusText, problem);
  }
  return data as T;
}

async function dbGet<T>(path: string): Promise<T> {
  const res = await fetch(path, { credentials: "same-origin", headers: { Accept: "application/json" } });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new DbError(res.status, (data as { message?: string }).message ?? res.statusText);
  return data as T;
}

const root = (t: string) => ["tenant", t, "db"] as const;

export const useDbInfo = (tenantId?: string) =>
  useQuery({ queryKey: [...root(tenantId ?? ""), "info"], queryFn: () => dbGet<DbInfo>(`/v1/tenants/${tenantId}/db`), enabled: !!tenantId });

export const useDbTables = (tenantId?: string) =>
  useQuery({
    queryKey: [...root(tenantId ?? ""), "tables"],
    queryFn: () => dbGet<{ tables: DbTable[] }>(`/v1/tenants/${tenantId}/db/tables`),
    enabled: !!tenantId,
  });

export type BrowseArgs = { schema: string; table: string; limit: number; offset: number; sort?: string; dir?: string; column?: string; op?: string; value?: string };

export const useDbBrowse = (tenantId: string, q: BrowseArgs | null) =>
  useQuery({
    queryKey: [...root(tenantId), "rows", q],
    queryFn: () => {
      const p = new URLSearchParams({ schema: q!.schema, limit: String(q!.limit), offset: String(q!.offset) });
      if (q!.sort) p.set("sort", q!.sort);
      if (q!.dir) p.set("dir", q!.dir);
      if (q!.column && q!.op) {
        p.set("column", q!.column);
        p.set("op", q!.op);
        if (q!.value != null) p.set("value", q!.value);
      }
      return dbGet<DbPage>(`/v1/tenants/${tenantId}/db/tables/${encodeURIComponent(q!.table)}?${p}`);
    },
    enabled: !!tenantId && !!q,
  });

export const useDbTemplates = (tenantId?: string) =>
  useQuery({
    queryKey: [...root(tenantId ?? ""), "templates"],
    queryFn: () => dbGet<{ templates: DbTemplate[] }>(`/v1/tenants/${tenantId}/db/templates`),
    enabled: !!tenantId,
  });

export const useDbSnippets = (tenantId?: string) =>
  useQuery({
    queryKey: [...root(tenantId ?? ""), "snippets"],
    queryFn: () => dbGet<{ snippets: DbSnippet[] }>(`/v1/tenants/${tenantId}/db/snippets`),
    enabled: !!tenantId,
  });

export const useDbAudit = (tenantId?: string) =>
  useQuery({
    queryKey: [...root(tenantId ?? ""), "audit"],
    queryFn: () => dbGet<{ entries: DbAudit[] }>(`/v1/tenants/${tenantId}/db/audit`),
    enabled: !!tenantId,
  });

export const useDbBackups = (tenantId?: string) =>
  useQuery({
    queryKey: [...root(tenantId ?? ""), "backups"],
    queryFn: () => dbGet<DbBackupList>(`/v1/tenants/${tenantId}/db/backups`),
    enabled: !!tenantId,
  });

function useRefresh(tenantId: string) {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: root(tenantId) });
  };
}

export function useApplyTemplate(tenantId: string, readOnly: boolean) {
  const refresh = useRefresh(tenantId);
  return useMutation({
    mutationFn: (id: string) => dbSend("POST", `/v1/tenants/${tenantId}/db/templates/${id}`, { read_only: readOnly }),
    onSuccess: () => {
      toast.success("Template created", "The tables are ready to browse.");
      refresh();
    },
    onError: (e: Error) => toast.error("Template not created", e.message),
  });
}

export function useCreateTable(tenantId: string, readOnly: boolean) {
  const refresh = useRefresh(tenantId);
  return useMutation({
    mutationFn: (body: { name: string; columns: { name: string; type: string; nullable: boolean; primary_key: boolean; default?: string }[] }) =>
      dbSend("POST", `/v1/tenants/${tenantId}/db/tables`, { ...body, read_only: readOnly }),
    onSuccess: () => {
      toast.success("Table created");
      refresh();
    },
    onError: (e: Error) => toast.error("Table not created", e.message),
  });
}

export function useAddColumn(tenantId: string, table: string, readOnly: boolean) {
  const refresh = useRefresh(tenantId);
  return useMutation({
    mutationFn: (body: { name: string; type: string; nullable: boolean; default?: string }) =>
      dbSend("POST", `/v1/tenants/${tenantId}/db/tables/${encodeURIComponent(table)}/columns`, { ...body, primary_key: false, read_only: readOnly }),
    onSuccess: () => {
      toast.success("Column added");
      refresh();
    },
    onError: (e: Error) => toast.error("Column not added", e.message),
  });
}

export function useSaveRow(tenantId: string, table: string, readOnly: boolean) {
  const refresh = useRefresh(tenantId);
  return useMutation({
    mutationFn: (body: { pk?: Record<string, unknown>; values: Record<string, unknown>; insert: boolean }) =>
      dbSend<{ row: Record<string, unknown> }>(body.insert ? "POST" : "PUT", `/v1/tenants/${tenantId}/db/tables/${encodeURIComponent(table)}/rows`,
        body.insert ? { values: body.values, read_only: readOnly } : { pk: body.pk, values: body.values, read_only: readOnly }),
    onSuccess: () => refresh(),
    onError: (e: Error) => toast.error("Row not saved", e.message),
  });
}

export function useDeleteRow(tenantId: string, table: string, readOnly: boolean) {
  const refresh = useRefresh(tenantId);
  return useMutation({
    mutationFn: (pk: Record<string, unknown>) =>
      dbSend("DELETE", `/v1/tenants/${tenantId}/db/tables/${encodeURIComponent(table)}/rows`, { pk, confirm: true, read_only: readOnly }),
    onSuccess: () => {
      toast.success("Row deleted");
      refresh();
    },
    onError: (e: Error) => toast.error("Row not deleted", e.message),
  });
}

export function useRunSQL(tenantId: string) {
  const refresh = useRefresh(tenantId);
  return useMutation({
    mutationFn: (body: { sql: string; read_only: boolean; confirm: boolean }) =>
      dbSend<DbQueryResult>("POST", `/v1/tenants/${tenantId}/db/query`, body),
    onSuccess: (r) => {
      if (r.kind !== "read") refresh();
    },
  });
}

export function useSaveSnippet(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; sql: string }) => dbSend<DbSnippet>("POST", `/v1/tenants/${tenantId}/db/snippets`, body),
    onSuccess: () => {
      toast.success("Snippet saved");
      void qc.invalidateQueries({ queryKey: [...root(tenantId), "snippets"] });
    },
    onError: (e: Error) => toast.error("Snippet not saved", e.message),
  });
}

export function useDeleteSnippet(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => dbSend("DELETE", `/v1/tenants/${tenantId}/db/snippets/${id}`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: [...root(tenantId), "snippets"] }),
    onError: (e: Error) => toast.error("Snippet not removed", e.message),
  });
}

export function useAskAI(tenantId: string) {
  return useMutation({
    mutationFn: (prompt: string) => dbSend<DbAsk>("POST", `/v1/tenants/${tenantId}/db/ask`, { prompt }),
  });
}

export function useCreateBackup(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (note: string) => dbSend<{ backup: DbBackup; message: string }>("POST", `/v1/tenants/${tenantId}/db/backups`, { note }),
    onSuccess: (r) => {
      toast.info("Backup requested", r.message);
      void qc.invalidateQueries({ queryKey: [...root(tenantId), "backups"] });
    },
    onError: (e: Error) => toast.error("Backup not requested", e.message),
  });
}

export function useRestoreBackup(tenantId: string) {
  return useMutation({
    mutationFn: (id: string) => dbSend("POST", `/v1/tenants/${tenantId}/db/backups/${id}/restore`),
    onError: (e: Error) => toast.info("Restore waiting", e.message),
  });
}
