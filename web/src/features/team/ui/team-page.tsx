import { UserPlus } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { useTenant } from "../../../lib/hooks/use-tenant";
import { useTenantTeam } from "../../../lib/hooks/use-tenant-team";
import { PageHeader } from "../../../template/page-header";
import { InviteMemberDialog } from "./invite-member-dialog";
import { MembersCard } from "./members-card";
import { TeamsCard } from "./teams-card";

/** Tenant Team page (CONSOLE-IA "Team"): members and roles through Nomen, plus teams. */
export function TeamPage() {
  const { tenant } = useTenant();
  const id = tenant?.tenant_id ?? "";
  const q = useTenantTeam(id);
  const [inviting, setInviting] = useState(false);
  const d = q.data;
  return (
    <>
      <PageHeader eyebrow={tenant?.name} title="Team" description="Who can use this tenant, what they may do, and how they're grouped." />
      {q.isError && <p role="alert" className="text-sm text-danger">{q.error.message}</p>}
      {d && (
        <div className="space-y-6">
          {(d.errors?.length ?? 0) > 0 && <p role="alert" className="text-sm text-warning">{(d.errors ?? []).join(" ")}</p>}
          <MembersCard tenantId={id} team={d} actions={d.can_manage && (
            <Button size="sm" variant="primary" tooltip="Invite someone by e-mail" onPress={() => setInviting(true)}><UserPlus className="size-4" /> Invite</Button>
          )} />
          <TeamsCard tenantId={id} team={d} />
          {inviting && <InviteMemberDialog tenantId={id} team={d} onClose={() => setInviting(false)} />}
        </div>
      )}
    </>
  );
}
