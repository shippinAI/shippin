export type DbColumn = {
  name: string;
  type: string;
  nullable: boolean;
  primary_key: boolean;
  default?: string;
};

export type DbTable = {
  schema: string;
  name: string;
  row_count: number | null;
  estimate: number;
  platform: boolean;
  comment?: string;
};

export type DbInfo = {
  database: string;
  role: string;
  host: string;
  port: number;
  sslmode: string;
  uri: string;
  login_ready: boolean;
  statement_timeout_ms: number;
  max_rows: number;
  password_note: string;
  hints: string[];
  snippets: { javascript: string; python: string; go: string };
};

export type DbPage = {
  schema: string;
  table: string;
  columns: DbColumn[];
  rows: Record<string, unknown>[];
  total: number;
  limit: number;
  offset: number;
  platform: boolean;
};

export type DbQueryResult = {
  columns: string[];
  rows: unknown[][];
  row_count: number;
  truncated: boolean;
  kind: string;
};

export type DbTemplate = {
  id: string;
  title: string;
  description: string;
  hint: string;
  tables: string[];
  statements: string[];
};

export type DbSnippet = { id: string; name: string; sql: string; updated_at?: string };

export type DbAudit = {
  id: string;
  actor_name: string;
  action: string;
  statement: string;
  kind: string;
  status: string;
  error: string;
  created_at: string;
  read_only: boolean;
};

export type DbBackup = {
  id: string;
  note?: string;
  status: string;
  created_at: string;
  actor: string;
};

export type DbBackupList = { backups: DbBackup[]; backend: string; message: string };

export type DbAsk = { status: string; hook: string; message: string; sql: string | null };

export const columnTypes: { id: string; label: string }[] = [
  { id: "text", label: "Text" },
  { id: "integer", label: "Integer" },
  { id: "bigint", label: "Big integer" },
  { id: "boolean", label: "Yes / no" },
  { id: "numeric", label: "Number" },
  { id: "timestamptz", label: "Date and time" },
  { id: "date", label: "Date" },
  { id: "uuid", label: "UUID" },
  { id: "jsonb", label: "JSON" },
];
