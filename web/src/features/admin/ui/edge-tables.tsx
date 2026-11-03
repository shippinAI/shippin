import { Badge } from "../../../lib/design/components/badge/badge";
import type { ColumnDef } from "../../../lib/design/components/data-table/data-table";
import type { CertInfo, EdgePort, EdgeRoute } from "../../../lib/api/admin-types";

const mono = "font-mono text-[12px]";
type Row<T> = T & { id: string };

export const routeColumns: ColumnDef<Row<EdgeRoute>>[] = [
  {
    id: "hosts", label: "Hosts", isRowHeader: true, render: (r) => (
      <ul>{r.hosts.length ? r.hosts.map((h) => <li key={h} className="font-medium text-fg">{h}</li>) : <li className="text-fg-subtle">(any host)</li>}</ul>
    ),
  },
  { id: "up", label: "Upstream", render: (r) => <span className={mono}>{r.upstreams.join(", ") || "—"}</span> },
  { id: "auth", label: "Auth", width: "120px", render: (r) => (r.forward_auth ? <Badge tone="primary">forward-auth</Badge> : <Badge>public</Badge>) },
  { id: "rid", label: "Route id", render: (r) => <span className={`text-fg-subtle ${mono}`}>{r.id.startsWith("route-") ? "—" : r.id}</span> },
];

export const certColumns: ColumnDef<Row<CertInfo>>[] = [
  { id: "host", label: "Host", isRowHeader: true, render: (c) => <span className="font-medium text-fg">{c.host}</span> },
  { id: "issuer", label: "Issuer", render: (c) => c.issuer || <span className="text-danger">{c.error}</span> },
  { id: "san", label: "Names", render: (c) => <span className={`block max-w-xs truncate ${mono}`} title={c.subjects.join(", ")}>{c.subjects.join(", ") || "—"}</span> },
  {
    id: "exp", label: "Expires", width: "190px", render: (c) => c.error ? <Badge tone="danger">unreachable</Badge> : (
      <span className="inline-flex items-center gap-2">
        <Badge tone={c.days_left < 7 ? "danger" : c.days_left < 21 ? "warning" : "success"}>{c.days_left}d</Badge>
        <span className="text-xs">{new Date(c.not_after).toLocaleDateString()}</span>
      </span>
    ),
  },
];

export const portColumns: ColumnDef<Row<EdgePort>>[] = [
  { id: "port", label: "Public port", isRowHeader: true, render: (p) => <span className={`text-fg ${mono}`}>{p.proto}/{p.port}</span> },
  { id: "dest", label: "Destination", render: (p) => <span className={mono}>{p.dest}:{p.dport}</span> },
  { id: "src", label: "Source", render: (p) => <span className={mono}>{p.source || "any"}</span> },
  { id: "owner", label: "Workspace", render: (p) => <span className={`text-fg-subtle ${mono}`}>{p.owner}</span> },
];
