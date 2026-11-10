import { Badge } from "../../../lib/design/components/badge/badge";
import type { ColumnDef } from "../../../lib/design/components/data-table/data-table";
import type { AdminApp, AdminIdP, AdminUser } from "../../../lib/api/admin-types";
import { timeAgo } from "../../../lib/format";
import { StatusBadge } from "./status-badge";

const mono = "font-mono text-xs";
const short = (s: string, p: string) => s.replace(p, "").toLowerCase().replaceAll("_", " ");

export const userColumns: ColumnDef<AdminUser>[] = [
  {
    id: "name", label: "User", isRowHeader: true, render: (u) => (
      <div className="min-w-0">
        <div className="truncate font-medium text-fg">{u.name || u.login}</div>
        <div className="truncate text-xs text-fg-subtle">{u.login}</div>
      </div>
    ),
  },
  {
    id: "email", label: "Email", render: (u) => u.email ? (
      <span className="inline-flex items-center gap-1.5">{u.email}{u.email_verified && <Badge tone="success">verified</Badge>}</span>
    ) : <span className="text-fg-subtle">—</span>,
  },
  { id: "kind", label: "Type", width: "100px", render: (u) => <Badge tone={u.kind === "machine" ? "info" : "neutral"}>{u.kind}</Badge> },
  { id: "org", label: "Organization", render: (u) => u.org || <span className={mono}>{u.org_id}</span> },
  { id: "state", label: "State", width: "110px", render: (u) => <StatusBadge status={u.state} /> },
  { id: "created", label: "Created", width: "100px", render: (u) => (u.created_at ? timeAgo(u.created_at) : "—") },
];

export const idpColumns: ColumnDef<AdminIdP>[] = [
  { id: "name", label: "Provider", isRowHeader: true, render: (p) => <span className="font-medium text-fg">{p.name}</span> },
  { id: "type", label: "Type", render: (p) => <span className="capitalize">{short(p.type, "PROVIDER_TYPE_")}</span> },
  { id: "id", label: "IdP id", render: (p) => <span className={mono}>{p.id}</span> },
  { id: "state", label: "State", width: "110px", render: (p) => <StatusBadge status={p.state} /> },
];

export const appColumns: ColumnDef<AdminApp>[] = [
  {
    id: "name", label: "Application", isRowHeader: true, render: (a) => (
      <div>
        <div className="font-medium text-fg">{a.name}</div>
        <div className="text-xs text-fg-subtle">{a.project} · {short(a.app_type, "OIDC_APP_TYPE_")}</div>
      </div>
    ),
  },
  { id: "client", label: "Client ID", render: (a) => <span className={mono}>{a.client_id || "—"}</span> },
  { id: "auth", label: "Auth", width: "110px", render: (a) => <span className="capitalize">{a.auth_method ? short(a.auth_method, "OIDC_AUTH_METHOD_TYPE_") : "—"}</span> },
  {
    id: "redirects", label: "Redirect URIs", render: (a) => (
      <ul className="space-y-0.5">
        {a.redirect_uris.length ? a.redirect_uris.map((u) => <li key={u} className={`max-w-md truncate ${mono}`} title={u}>{u}</li>) : <li className="text-fg-subtle">—</li>}
      </ul>
    ),
  },
];
