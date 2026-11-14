import { apiGet, apiSend, ApiError } from "../../lib/api/client";
import type { RunnerProject } from "./model";

export type ProfileField = { type: "integer" | "string"; description: string; minimum?: number; maximum?: number; default?: number; pattern?: string };
export type ProfileSchema = { properties: Record<string, ProfileField>; required: string[]; "x-contract-version": string; "x-enforcement-status": string };
export type ProfileTemplate = { id: string; project: string; revision: number; schema_version: number; template: Record<string, string | number>; worker_applied: string };
const fieldNames = ["vcpu_count", "memory_mib", "disk_gib", "timeout_seconds", "image_sha256", "kernel_sha256", "runner_version"];
const identifier = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/;
export function grantedProfiles(project: RunnerProject) {
  return project.scopes.includes("profiles:read") ? [...new Set((project.profiles ?? []).filter((id) => typeof id === "string" && identifier.test(id)))] : [];
}
function base(tenant: string, project: RunnerProject, profile: string) {
  if (tenant !== project.tenant_id || !identifier.test(project.id) || !grantedProfiles(project).includes(profile)) throw new ApiError(403, "Profile access required");
  return `/v1/projects/${project.id}`;
}
export function validateSchema(schema: ProfileSchema) {
  if (schema["x-contract-version"] !== "1" || schema["x-enforcement-status"] !== "unverified" || !schema.properties || !Array.isArray(schema.required)) throw new Error("Unsupported profile schema");
  if (Object.keys(schema.properties).length !== fieldNames.length || fieldNames.some((key) => !Object.hasOwn(schema.properties, key))) throw new Error("Unsupported profile fields");
  for (const field of Object.values(schema.properties)) {
    if (!["integer", "string"].includes(field.type) || typeof field.description !== "string") throw new Error("Unsupported field");
    if (field.type === "integer" && (!Number.isSafeInteger(field.minimum) || !Number.isSafeInteger(field.maximum))) throw new Error("Invalid bounds");
    if (field.pattern) new RegExp(field.pattern);
  }
  return schema;
}
export function draftOf(template: ProfileTemplate | null) {
  return Object.fromEntries(Object.entries(template?.template ?? {}).map(([key, value]) => [key, String(value)]));
}
export function profilePayload(schema: ProfileSchema, draft: Record<string, string>) {
  validateSchema(schema);
  const body: Record<string, string | number> = {};
  const errors: Record<string, string> = {};
  for (const [key, field] of Object.entries(schema.properties)) {
    const value = draft[key] ?? "";
    if (value === "") { if (schema.required.includes(key)) errors[key] = "required"; continue; }
    if (field.type === "integer") {
      const n = Number(value);
      if (!/^-?\d+$/.test(value) || !Number.isSafeInteger(n) || n < field.minimum! || n > field.maximum!) errors[key] = "bounds";
      else body[key] = n;
    } else if (field.pattern && !new RegExp(field.pattern).test(value)) errors[key] = "format";
    else body[key] = value;
  }
  return { body, errors };
}
function checkedTemplate(data: ProfileTemplate, project: RunnerProject, profile: string, schema: ProfileSchema) {
  if (data.id !== profile || data.project !== project.id || !Number.isSafeInteger(data.revision) || data.revision < 1 || data.schema_version !== 1 || data.worker_applied !== "unreported") throw new Error("Invalid template response");
  if (!data.template || typeof data.template !== "object" || Array.isArray(data.template) || Object.entries(data.template).some(([key, value]) => !Object.hasOwn(schema.properties, key) || (schema.properties[key].type === "integer" ? typeof value !== "number" : typeof value !== "string")) || Object.keys(profilePayload(schema, draftOf(data)).errors).length) throw new Error("Invalid template fields");
  return data;
}
export async function loadProfile(tenant: string, project: RunnerProject, profile: string) {
  const path = base(tenant, project, profile);
  const schema = validateSchema(await apiGet<ProfileSchema>(path + "/profiles/schema"));
  let template: ProfileTemplate | null;
  try { template = checkedTemplate(await apiGet<ProfileTemplate>(path + "/profile-templates/" + profile), project, profile, schema); }
  catch (error) { if (error instanceof ApiError && error.status === 404) template = null; else throw error; }
  return { schema, template };
}
export async function saveProfile(tenant: string, project: RunnerProject, profile: string, revision: number, schema: ProfileSchema, draft: Record<string, string>) {
  const path = base(tenant, project, profile);
  if (!project.scopes.includes("profiles:write")) throw new ApiError(403, "Profile write access required");
  if (!Number.isSafeInteger(revision) || revision < 0) throw new Error("Invalid revision");
  const payload = profilePayload(schema, draft);
  if (Object.keys(payload.errors).length) throw new Error("Invalid profile fields");
  return checkedTemplate(await apiSend<ProfileTemplate>("POST", path + "/profile-templates/" + profile + "/revisions/" + revision, payload.body), project, profile, schema);
}
