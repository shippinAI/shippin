import { apiGet, ApiError } from "../../lib/api/client";

export type RunnerProject = { id: string; tenant_id: string; name: string; scopes: string[] };
export type RunnerRun = { id: string; project: string; repository: string; profile: string; state: string; projected_at: string; cancel_requested: boolean };
export type Runs = { items: RunnerRun[]; consistency: string; next_cursor?: string };
export type Connections = { github: { state: string }; execution_hosts: { state: string; workers: { id: string; state: string; repository: string; last_seen?: string }[]; truncated: boolean } };
const identifier = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/;
export const runnerTabs = ["overview", "runs", "connections", "configuration"] as const;
export function runnerTab(tab?: string) { return runnerTabs.find((v) => v === tab) ?? "overview"; }
export function runnerHref(tenant: string, project: string, tab: string) {
  return `/t/${encodeURIComponent(tenant)}/runners/${encodeURIComponent(project)}/${runnerTab(tab)}`;
}
export function projectsForTenant(projects: RunnerProject[], tenant: string) {
  return projects.filter((p) => p && p.tenant_id === tenant && typeof p.id === "string" && identifier.test(p.id) && typeof p.name === "string" && Array.isArray(p.scopes) && p.scopes.every((s) => typeof s === "string"));
}
export async function loadProjects(tenant: string) {
  const result = await apiGet<{ projects: RunnerProject[] | null }>("/v1/runner/projects");
  if (result.projects !== null && !Array.isArray(result.projects)) throw new Error("Invalid project response");
  return projectsForTenant(result.projects ?? [], tenant);
}
export function canRead(project: RunnerProject, resource: "runs" | "connectors") {
  return project.scopes.includes(resource === "runs" ? "runs:read" : "workers:manage");
}
export async function loadResource<T>(tenant: string, project: RunnerProject, resource: "runs" | "connectors"): Promise<T> {
  if (project.tenant_id !== tenant || !identifier.test(project.id) || !canRead(project, resource)) throw new ApiError(403, "Project access required");
  const data = await apiGet<Runs & Connections>(`/v1/projects/${project.id}/${resource}`);
  if (resource === "runs" && (data.consistency !== "eventual" || (data.items !== null && !Array.isArray(data.items)) || data.items?.some((r) => r.project !== project.id))) throw new Error("Invalid run response");
  if (resource === "connectors" && (!data.github || !data.execution_hosts || (data.execution_hosts.workers !== null && !Array.isArray(data.execution_hosts.workers)))) throw new Error("Invalid connection response");
  return data as T;
}
export function errorKey(error: unknown) {
  if (error instanceof ApiError && error.status === 401) return "runners.signIn" as const;
  if (error instanceof ApiError && error.status === 403) return "runners.denied" as const;
  return "runners.unavailable" as const;
}
