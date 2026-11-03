// CP-P0b orgs (/v1/orgs, /v1/ops/*). One Nomen identity, several orgs.
import type { Server } from "./server-types";

export type OrgType = "operations" | "customer";

export type OrgItem = {
  id: string; slug: string; type: OrgType; name: string;
  roles: string[]; has_devspace: boolean; tenant_id?: string;
};

export type OrgList = { items: OrgItem[]; staff_roles: string[] };

export type OpsServerList = { items: Server[]; can_admin: boolean };
