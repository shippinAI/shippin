export type OrgMember = { user_id: string; name: string; email: string; roles: string[] };
export type TeamMember = { user_id: string; role: "member" | "maintainer" };
export type Team = { id: string; name: string; description: string; created_at: string; members: TeamMember[] };
export type TenantTeam = {
  members: OrgMember[];
  teams: Team[];
  /** IdP org roles this page may grant (from config roles.org). */
  roles: string[];
  owner_role: string;
  can_manage: boolean;
  is_owner: boolean;
  errors: string[] | null;
};
export type InviteBody = { email: string; first_name: string; last_name: string; roles: string[] };
