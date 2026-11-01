import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { Team, TeamSettings } from "../api/team-types";
import { toast } from "../design/components/toast/toast-queue";

export const useTeam = () =>
  useQuery({ queryKey: ["admin", "team"], queryFn: () => apiGet<Team>("/v1/operator/team"), refetchInterval: 30_000, retry: false });

type Sent = { status?: string; id?: string };

/** One Team change: toasts the outcome; 202 means it waits for a second Owner. */
function useTeamSend<V>(build: (v: V) => [method: "POST" | "PUT" | "DELETE", path: string, body?: unknown], done: string, failed: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: V) => {
      const [m, p, b] = build(v);
      return apiSend<Sent>(m, p, b);
    },
    onSuccess: (d) =>
      d?.status === "pending_approval" ? toast.info("Waiting for a second Owner", "The two-person rule applies to this change.") : toast.success(done),
    onError: (e) => toast.error(failed, e.message),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["admin"] });
      void qc.invalidateQueries({ queryKey: ["me"] });
    },
  });
}

export const useSetRoles = () =>
  useTeamSend((v: { id: string; roles: string[]; reason: string }) => ["PUT", `/v1/operator/team/members/${v.id}/roles`, { roles: v.roles, reason: v.reason }], "Roles saved", "Roles not changed");

export const useSetActive = () =>
  useTeamSend((v: { id: string; state: "deactivate" | "reactivate"; reason: string }) => ["POST", `/v1/operator/team/members/${v.id}/${v.state}`, { reason: v.reason }], "Account updated", "Account not changed");

export type InviteBody = { email: string; first_name: string; last_name: string; roles: string[]; reason: string };
export const useInvite = () =>
  useTeamSend((v: InviteBody) => ["POST", "/v1/operator/team/members", v], "Invitation sent", "Couldn't invite");

export type RoleBody = { key: string; name: string; description: string; permissions: string[]; reason: string };
export const useSaveRole = (create: boolean) =>
  useTeamSend((v: RoleBody) => (create ? ["POST", "/v1/operator/team/roles", v] : ["PUT", `/v1/operator/team/roles/${v.key}`, { ...v, key: undefined }]), create ? "Role created" : "Role saved", "Role not saved");

export const useDeleteRole = () => useTeamSend((key: string) => ["DELETE", `/v1/operator/team/roles/${key}`], "Role deleted", "Role not deleted");

export const useSaveSettings = () =>
  useTeamSend((v: TeamSettings & { reason: string }) => ["PUT", "/v1/operator/team/settings", v], "Team settings saved", "Settings not saved");

export const useDecideRequest = () =>
  useTeamSend((v: { id: string; decision: "approve" | "reject" | "cancel" }) => ["POST", `/v1/operator/team/requests/${v.id}/${v.decision}`], "Request decided", "Couldn't decide");
