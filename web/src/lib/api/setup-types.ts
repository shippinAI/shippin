export type SetupState = "ok" | "todo" | "off" | "error";
export type SetupCheck = { id: string; label: string; state: SetupState; detail?: string; project_id?: string; fix?: string };
export type SetupStatus = { tenant_id: string; checks: SetupCheck[]; done: number; total: number };

export type TenantMember = { user_id: string; name: string; email: string; roles: string[] };
export type TenantOwner = { account_id: string; email: string; since: string };
export type TenantProject = { id: string; name: string; state: string; workspaces: number };
export type AdminTenantDetail = {
  org: { id: string; name: string };
  members: TenantMember[];
  owners: TenantOwner[];
  projects: TenantProject[];
  teams: { id: string; name: string; members: { user_id: string; role: string }[] }[] | null;
  errors: string[];
};
