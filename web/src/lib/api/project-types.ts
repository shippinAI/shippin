// Mirrors internal/projects (M1). Boundaries are never exposed to tenants; only `placed`.
export type Environment = { id: string; name: string };

export type Project = {
  id: string;
  workspace_id: string;
  name: string;
  state: "pending" | "ready" | "error" | "deleting";
  placed: boolean;
  created_at: string;
  environments: Environment[];
  workspaces: number;
};
