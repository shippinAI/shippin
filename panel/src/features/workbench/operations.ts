import type { AppMode, MeIdentity, OpenApiOperation, RuntimeConfig } from "../../shared/contracts";

const customerTags = new Set(["me", "folders", "workspaces", "jobs", "members", "settings", "secrets", "identity", "rrflow", "audit"]);
const operatorTags = new Set(["me", "platform", "tenants", "jobs", "settings", "audit"]);
const actionByMethod: Record<string, string> = { GET: "read", POST: "create", PUT: "update", PATCH: "update", DELETE: "delete" };

export function hasOperatorRole(identity: MeIdentity | null): boolean {
  return identity?.platform_roles?.some((role) => ["op_support", "op_admin", "op_superadmin"].includes(role)) ?? false;
}

export function getPathParameters(path: string): string[] {
  return (path.match(/\{([^}]+)\}/g) ?? []).map((match) => match.slice(1, -1));
}

export function isOperationAllowedForMode(operation: OpenApiOperation, mode: AppMode, identity: MeIdentity | null): boolean {
  const tag = operation.tags[0] ?? "untagged";
  return mode === "customer" ? customerTags.has(tag) : hasOperatorRole(identity) && operatorTags.has(tag);
}

export function getCapabilityFromOperation(operation: OpenApiOperation, config: RuntimeConfig | null): string {
  return config?.capabilityHints?.[operation.operationId] ||
    `${operation.tags[0] ?? "resource"}:${actionByMethod[operation.method.toUpperCase()] ?? "manage"}`;
}

export function isCapabilityAllowed(operation: OpenApiOperation, config: RuntimeConfig | null, identity: MeIdentity | null): boolean {
  const capabilities = identity?.capabilities;
  if (!capabilities) return true;
  const key = getCapabilityFromOperation(operation, config);
  return key in capabilities ? Boolean(capabilities[key]) : true;
}

/** Resolve each API path placeholder as exactly one unambiguous segment. */
export function resolveOperationPath(path: string, values: Record<string, string>): string {
  return path.replace(/\{([^}]+)\}/g, (_, key: string) => {
    const value = values[key]?.trim();
    if (!value) throw new Error(`Missing required path parameter: ${key}`);
    if (value === "." || value === ".." || /[\\/%?#\x00-\x1f\x7f]/.test(value)) {
      throw new Error(`Invalid path parameter: ${key}. Enter a single unescaped path value.`);
    }
    try {
      return encodeURIComponent(value);
    } catch {
      throw new Error(`Invalid path parameter: ${key}.`);
    }
  });
}
