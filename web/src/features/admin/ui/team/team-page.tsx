import { ShieldAlert, ShieldCheck, UserCog, Users } from "lucide-react";
import { Tabs } from "../../../../lib/design/components/tabs/tabs";
import { useAdminAccess } from "../../../../lib/hooks/use-admin";
import { useTeam } from "../../../../lib/hooks/use-team";
import { PageHeader } from "../../../../template/page-header";
import { QueryView } from "../query-view";
import { StatCard } from "../stat-card";
import { GrantsPanel } from "./grants-panel";
import { MembersPanel } from "./members-panel";
import { RequestsCard } from "./requests-card";
import { RolesPanel } from "./roles-panel";
import { SettingsPanel } from "./settings-panel";
import { useIdpName } from "../../../../lib/hooks/use-idp-name";

/** Team: staff, roles (data, with permission checkboxes) and team settings. */
export function TeamPage() {
  const idp = useIdpName();
  const q = useTeam();
  const perms = useAdminAccess().data?.staff?.permissions ?? [];
  const can = (p: string) => perms.includes(p);
  const t = q.data;
  const staff = t?.members.filter((m) => m.roles.length);
  return (
    <>
      <PageHeader eyebrow="Provider" title="Team" description={idp === "your identity provider" ? "Who can use this panel and what each role may do. Every change is audited." : `Who works on Shippin and what each role may do. Every change is audited before it reaches ${idp}.`} />
      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard label="Staff" value={staff?.length} icon={Users} hue="var(--color-primary)" hint="Members with at least one role" />
        <StatCard label="Owners" value={t?.owners} icon={UserCog} hue="var(--color-warning)" hint={t && t.owners < 2 ? "Add a second Owner for two-person approvals" : "Active"} />
        <StatCard label="MFA missing" value={staff?.filter((m) => m.kind === "human" && !m.mfa).length} icon={ShieldAlert} hue="var(--color-danger)" hint="Their staff permissions are off" />
        <StatCard label="Idle" value={staff?.filter((m) => m.idle).length} icon={ShieldCheck} hue="var(--color-fg-muted)" hint={t ? `No console activity in ${t.settings.idle_days} days` : undefined} />
      </div>
      {t?.errors.length ? <div className="mb-3 rounded-lg border border-warning/30 bg-warning/10 px-4 py-2 text-sm text-warning">{t.errors.join(" · ")}</div> : null}
      <QueryView isPending={q.isPending} error={q.error} onRetry={() => void q.refetch()} rows={4}>
        {() => t && (
          <>
            <RequestsCard team={t} canWrite={can("team.write")} />
            <Tabs label="Team sections" tabs={[
              { id: "members", label: `Members (${t.members.length})`, content: <MembersPanel team={t} canWrite={can("team.write")} /> },
              { id: "roles", label: `Roles (${t.roles.length})`, content: <RolesPanel team={t} canEdit={can("team.roles")} /> },
              { id: "grants", label: "Direct grants", content: <GrantsPanel team={t} canWrite={can("team.write")} /> },
              { id: "settings", label: "Settings", content: <SettingsPanel team={t} canEdit={can("team.settings")} /> },
            ]} />
          </>
        )}
      </QueryView>
    </>
  );
}
