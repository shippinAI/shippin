import { UserPlus } from "lucide-react";
import { useState } from "react";
import { Card } from "../../../../lib/design/components/card/card";
import { Button } from "../../../../lib/design/components/button/button";
import { DataTable } from "../../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../../lib/design/components/empty-state/empty-state";
import type { Team } from "../../../../lib/api/team-types";
import { TableToolbar, matches } from "../table-toolbar";
import { InviteDialog } from "./invite-dialog";
import { memberColumns } from "./member-columns";
import { MemberDrawer } from "./member-drawer";

/** Staff list with search, invite and a drawer per member. */
export function MembersPanel({ team, canWrite }: { team: Team; canWrite: boolean }) {
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState<string>();
  const [inviting, setInviting] = useState(false);
  const rows = team.members.filter((m) => matches(search, m.name, m.email, m.login, ...m.roles));
  return (
    <Card className="overflow-hidden">
      <TableToolbar label="staff" value={search} onChange={setSearch} count={rows.length}
        actions={canWrite && <Button variant="primary" size="sm" tooltip="Invite anyone by e-mail" onPress={() => setInviting(true)}><UserPlus className="size-4" /> Invite</Button>} />
      <DataTable label="Staff" columns={memberColumns(team.roles)} rows={rows} onRowAction={setOpen}
        empty={<EmptyState title="No staff yet" description="Invite someone by e-mail to get started." />} />
      <MemberDrawer team={team} member={team.members.find((m) => m.id === open)} canWrite={canWrite} onClose={() => setOpen(undefined)} />
      {inviting && <InviteDialog roles={team.roles} onClose={() => setInviting(false)} />}
    </Card>
  );
}
