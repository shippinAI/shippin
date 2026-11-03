import { AppWindow, Fingerprint, KeyRound, Users } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { useAdminIdentity } from "../../../lib/hooks/use-admin";
import { PageHeader } from "../../../template/page-header";
import { appColumns, idpColumns, userColumns } from "./identity-tables";
import { QueryView } from "./query-view";
import { StatCard } from "./stat-card";
import { TableToolbar, matches } from "./table-toolbar";
import { useIdpName } from "../../../lib/hooks/use-idp-name";

export function IdentityPage() {
  const idp = useIdpName();
  const q = useAdminIdentity();
  const [search, setSearch] = useState("");
  const d = q.data;
  const users = (d?.users ?? []).filter((u) => matches(search, u.name, u.login, u.email, u.org, u.kind));
  const view = (body: () => ReactNode) => <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()} rows={3}>{body}</QueryView>;
  return (
    <>
      <PageHeader eyebrow="Provider" title="Users & sign-in" description={<>{idp} at <span className="font-mono">{d?.issuer ?? "…"}</span>. Read-only; secrets are never shown.</>} />
      {d?.errors.length ? <div className="mb-3 rounded-lg border border-warning/30 bg-warning/10 px-4 py-2 text-[13px] text-warning">{d.errors.join(" · ")}</div> : null}
      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard label="Users" value={d ? d.users.filter((u) => u.kind === "human").length : undefined} icon={Users} hue="var(--color-hue-identity)" hint="Human accounts" />
        <StatCard label="Service accounts" value={d ? d.users.filter((u) => u.kind === "machine").length : undefined} icon={KeyRound} hue="var(--color-hue-secrets)" hint="Machine users" />
        <StatCard label="Identity providers" value={d?.idps.length} icon={Fingerprint} hue="var(--color-primary)" hint={d?.idps.map((p) => p.name).join(", ")} />
        <StatCard label="Applications" value={d?.apps.length} icon={AppWindow} hue="var(--color-hue-edge)" hint="Platform org" />
      </div>
      <div className="space-y-5">
        <Card className="overflow-hidden">
          <CardHeader title="Identity providers" description="Instance-level; shown on the hosted login page." />
          {view(() => <DataTable label="Identity providers" dense columns={idpColumns} rows={d?.idps ?? []} empty={<EmptyState title="No identity providers" description={`Add Google or GitHub in ${idp}.`} />} />)}
        </Card>
        <Card className="overflow-hidden">
          <CardHeader title="OIDC applications" description="Client ids and redirect URIs (no secrets)." />
          {view(() => <DataTable label="Applications" dense columns={appColumns} rows={d?.apps ?? []} empty={<EmptyState title="No applications" description="No OIDC apps in the platform org." />} />)}
        </Card>
      </div>
      <Card className="mt-5 overflow-hidden">
        <CardHeader title="Users" description="All organizations." />
        <TableToolbar label="users" value={search} onChange={setSearch} count={users.length} />
        {view(() => <DataTable label="Users" dense columns={userColumns} rows={users} empty={<EmptyState title="No matching users" description="Try another name, email or org." />} />)}
      </Card>
    </>
  );
}
