import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { InviteBody, TenantTeam } from "../api/tenant-team-types";
import { toast } from "../design/components/toast/toast-queue";

const key = (t: string) => ["tenant-team", t];

export const useTenantTeam = (tenantId: string) =>
  useQuery({ queryKey: key(tenantId), queryFn: () => apiGet<TenantTeam>(`/v1/tenants/${tenantId}/team`), retry: false });

/** One mutation per Team-page write; every one is audited server-side before Nomen is called. */
function useTeamWrite<V>(tenantId: string, run: (v: V) => Promise<unknown>, done: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: run,
    onSuccess: () => {
      toast.success(done);
      return qc.invalidateQueries({ queryKey: key(tenantId) });
    },
    onError: (e) => toast.error("Not changed", e.message),
  });
}

export function useTeamWrites(tenantId: string) {
  const b = `/v1/tenants/${tenantId}`;
  return {
    invite: useTeamWrite(tenantId, (v: InviteBody) => apiSend("POST", `${b}/members`, v), "Invite sent"),
    setRoles: useTeamWrite(tenantId, (v: { user: string; roles: string[] }) => apiSend("PATCH", `${b}/members/${v.user}`, { roles: v.roles }), "Roles updated"),
    remove: useTeamWrite(tenantId, (user: string) => apiSend("DELETE", `${b}/members/${user}`), "Member removed"),
    createTeam: useTeamWrite(tenantId, (v: { name: string; description: string }) => apiSend("POST", `${b}/teams`, v), "Team created"),
    deleteTeam: useTeamWrite(tenantId, (team: string) => apiSend("DELETE", `${b}/teams/${team}`), "Team deleted"),
    addToTeam: useTeamWrite(tenantId, (v: { team: string; user: string; role: string }) => apiSend("PUT", `${b}/teams/${v.team}/members/${v.user}`, { role: v.role }), "Team updated"),
    removeFromTeam: useTeamWrite(tenantId, (v: { team: string; user: string }) => apiSend("DELETE", `${b}/teams/${v.team}/members/${v.user}`), "Team updated"),
  };
}
