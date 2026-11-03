export type Perm = { key: string; section: string; label: string; high_risk: boolean; reserved: boolean; grantable?: boolean };

/** One permission granted to one principal directly (cloud-IAM style; Owner grants, audited). */
export type PermGrant = { subject: string; permission: string; reason: string; granted_by: string; granted_at: string };
export type PermGrants = { items: PermGrant[]; grantable: Perm[] };

export type StaffRole = { key: string; name: string; description: string; permissions: string[]; preset: boolean; updated_at: string };

export type TeamMember = {
  id: string;
  name: string;
  email: string;
  login: string;
  state: "active" | "inactive" | "initial" | "locked" | string;
  kind: "human" | "machine" | string;
  roles: string[];
  mfa: boolean;
  last_seen?: string;
  idle: boolean;
  owner: boolean;
  self: boolean;
};

export type TeamSettings = {
  mode: "legacy" | "shadow" | "enforce";
  legacy_fallback: boolean;
  two_person: boolean;
  idle_days: number;
  breakglass_approval: boolean;
  approval_minutes: number;
  sole_owner_activation: boolean;
};

export type TeamRequest = {
  id: string;
  kind: "roles" | "invite" | "role";
  target: string;
  target_name: string;
  payload: { roles?: string[]; permissions?: string[]; email?: string; name?: string };
  reason: string;
  requested_by: string;
  requested_name: string;
  created_at: string;
};

export type Team = {
  members: TeamMember[];
  roles: StaffRole[];
  catalog: Perm[];
  settings: TeamSettings;
  requests: TeamRequest[];
  owners: number;
  two_person_active: boolean;
  mode_override: string;
  self: string;
  errors: string[];
};

export type StaffSummary = {
  mode: string;
  permissions: string[];
  roles: string[];
  mfa: boolean;
  mfa_required: boolean;
  legacy_fallback: boolean;
  unavailable?: boolean;
};
