import type { DbTable } from "../../lib/api/database-types";

/** Starter tables hostd creates for platform services. They live in the tenant database, not in Vaultix. */
export const platformTables = new Set(["secrets_refs", "idp_users", "idp_groups", "dns_zones", "dns_records"]);

export function isUserTable(t: Pick<DbTable, "schema" | "name" | "platform">): boolean {
  return t.schema === "public" && !t.platform && !platformTables.has(t.name);
}

export function userTables<T extends Pick<DbTable, "schema" | "name" | "platform">>(tables: T[]): T[] {
  return tables.filter(isUserTable);
}

export function rowCountLabel(t: Pick<DbTable, "row_count" | "estimate">): string {
  if (t.row_count != null) {
    const n = t.row_count;
    return n === 1 ? "1 row" : `${n.toLocaleString()} rows`;
  }
  if (t.estimate > 0) return `about ${Math.round(t.estimate).toLocaleString()} rows`;
  return "empty";
}

/** A one-line explanation of what the statement in the editor will do. */
export function hintForSQL(sql: string): string {
  const s = sql.trim().toLowerCase().replace(/^\(+/, "");
  if (!s) return "Write a SELECT to read rows. One statement at a time.";
  if (s.startsWith("select") || s.startsWith("with") || s.startsWith("explain") || s.startsWith("show")) {
    return "This reads data. It won't change rows unless you turn it into an INSERT, UPDATE, or DELETE.";
  }
  if (s.startsWith("insert")) return "This adds rows. Read-only mode will refuse it.";
  if (s.startsWith("update")) return "This changes existing rows. Check the WHERE clause so you don't update the whole table.";
  if (s.startsWith("delete") || s.startsWith("drop") || s.startsWith("truncate")) {
    return "This can remove data. You'll be asked to confirm before it runs.";
  }
  if (s.startsWith("create") || s.startsWith("alter") || s.startsWith("comment")) {
    return "This changes the shape of the database. Dropping a column still asks you to confirm.";
  }
  return "Run one statement. It only sees this organization's database.";
}

export function cellText(value: unknown): string {
  if (value == null) return "";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

export function containsFilter(value: string): { op: string; value: string } {
  return { op: "ilike", value: `%${value.replace(/[%_\\]/g, (c) => `\\${c}`)}%` };
}
