import { Globe, Lock, PlugZap, Route } from "lucide-react";
import type { ReactNode } from "react";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { useAdminEdge } from "../../../lib/hooks/use-admin";
import { PageHeader } from "../../../template/page-header";
import { certColumns, portColumns, routeColumns } from "./edge-tables";
import { QueryView } from "./query-view";
import { StatCard } from "./stat-card";

const Err = ({ msg }: { msg?: string }) => (msg ? <div className="px-5 py-3 text-sm text-danger">{msg}</div> : null);

export function EdgePage() {
  const q = useAdminEdge();
  const d = q.data;
  const routes = (d?.routes?.items ?? []).map((r, i) => ({ ...r, id: r.id || `route-${i}` }));
  const certs = (d?.certs ?? []).map((c) => ({ ...c, id: c.host }));
  const ports = (d?.ports?.items ?? []).map((p) => ({ ...p, id: `${p.proto}-${p.port}` }));
  const soon = certs.filter((c) => !c.error && c.days_left < 21).length;
  const view = (body: () => ReactNode) => <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()} rows={3}>{body}</QueryView>;
  return (
    <>
      <PageHeader eyebrow="Provider" title="Edge" description={<>Caddy routes, TLS as served by the edge, and port forwards on <span className="font-mono">{d?.ports?.public_ip ?? "…"}</span>.</>} />
      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard label="Routes" value={d ? routes.length : undefined} icon={Route} hue="var(--color-hue-edge)" hint={d ? `${routes.filter((r) => r.forward_auth).length} behind forward-auth` : undefined} />
        <StatCard label="Domains" value={d ? new Set(routes.flatMap((r) => r.hosts)).size : undefined} icon={Globe} hue="var(--color-primary)" />
        <StatCard label="Certificates" value={d ? certs.length : undefined} icon={Lock} hue="var(--color-success)" hint={d ? (soon ? `${soon} expire within 21 days` : "None expiring soon") : undefined} />
        <StatCard label="Port forwards" value={d ? ports.length : undefined} icon={PlugZap} hue="var(--color-hue-data)" hint={d?.ports ? `${d.ports.min}–${d.ports.max} SSH range` : undefined} />
      </div>
      <Card className="overflow-hidden">
        <CardHeader title="Certificates" description="Leaf certificate the edge serves for each name (SNI probe)." />
        {view(() => <DataTable label="Certificates" dense columns={certColumns} rows={certs} empty={<EmptyState title="No certificates" description="No hosts to probe." />} />)}
      </Card>
      <Card className="mt-5 overflow-hidden">
        <CardHeader title="Routes & domains" description="Live Caddy config via the admin API (read-only)." />
        <Err msg={d?.routes_error} />
        {view(() => <DataTable label="Routes" dense columns={routeColumns} rows={routes} empty={<EmptyState title="No routes" description="The edge has no HTTP routes." />} />)}
      </Card>
      <Card className="mt-5 overflow-hidden">
        <CardHeader title="Port forwards" description="Shared endpoint → workspace (edge nftables DNAT)." />
        <Err msg={d?.ports_error} />
        {view(() => <DataTable label="Port forwards" dense columns={portColumns} rows={ports} empty={<EmptyState title="No forwards" description="No SSH or published ports yet." />} />)}
      </Card>
    </>
  );
}
