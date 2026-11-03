import type { AdminTenantDetail } from "../../../lib/api/setup-types";
import { useAdminTenantDetail } from "../../../lib/hooks/use-setup";
import { useIdpName } from "../../../lib/hooks/use-idp-name";

function Section({ title, empty, children }: { title: string; empty: boolean; children: React.ReactNode }) {
  return (
    <div className="mx-5 mt-2 rounded-lg border border-line bg-raised/50 px-4 py-3">
      <div className="text-[13px] font-medium text-fg">{title}</div>
      {empty ? <p className="mt-1 text-xs text-fg-subtle">None.</p> : <ul className="mt-2 space-y-1.5 text-[13px]">{children}</ul>}
    </div>
  );
}

/** Tenant org, members with org roles (from the IdP), Shippin owner rows and projects. */
export function TenantPeople({ tenantId }: { tenantId: string }) {
  const idp = useIdpName();
  const q = useAdminTenantDetail(tenantId);
  const d: AdminTenantDetail | undefined = q.data;
  if (q.isError) return <p className="mx-5 mt-2 text-xs text-danger">Tenant detail unavailable: {q.error.message}</p>;
  if (!d) return null;
  return (
    <>
      {d.errors.length > 0 && <p role="alert" className="mx-5 mt-2 text-xs text-warning">{d.errors.join("; ")}</p>}
      <Section title={`${idp} org members (${d.members.length})`} empty={d.members.length === 0}>
        {d.members.map((m) => (
          <li key={m.user_id} className="flex justify-between gap-3">
            <span className="truncate">{m.name || m.user_id} <span className="text-fg-subtle">{m.email}</span></span>
            <span className="shrink-0 font-mono text-[11px] text-fg-muted">{m.roles.join(", ")}</span>
          </li>
        ))}
      </Section>
      <Section title={`Teams (${d.teams?.length ?? 0})`} empty={!d.teams?.length}>
        {(d.teams ?? []).map((t) => (
          <li key={t.id} className="flex justify-between gap-3">
            <span className="truncate font-medium">{t.name}</span>
            <span className="text-fg-subtle">{t.members.length} member{t.members.length === 1 ? "" : "s"}</span>
          </li>
        ))}
      </Section>
      <Section title={`Shippin owners (${d.owners.length})`} empty={d.owners.length === 0}>
        {d.owners.map((o) => (
          <li key={o.account_id} className="flex justify-between gap-3">
            <span className="truncate">{o.email || o.account_id}</span>
            <span className="text-fg-subtle">since {new Date(o.since).toLocaleDateString()}</span>
          </li>
        ))}
      </Section>
      <Section title={`Projects (${d.projects.length})`} empty={d.projects.length === 0}>
        {d.projects.map((p) => (
          <li key={p.id} className="flex justify-between gap-3">
            <span className="truncate font-medium">{p.name}</span>
            <span className="text-fg-subtle">{p.state} · {p.workspaces} devspace{p.workspaces === 1 ? "" : "s"}</span>
          </li>
        ))}
      </Section>
    </>
  );
}
