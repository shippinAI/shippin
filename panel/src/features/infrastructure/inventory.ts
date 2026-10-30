export type ComputeNode = {
  id: string;
  name: string;
  provider: "incus" | "proxmox";
  status: "enrolling" | "verifying" | "ready" | "degraded" | "offline" | "revoked";
  schedulable: boolean;
  capabilities: string[];
  last_seen_at?: string | null;
  readiness_reasons?: string[];
};

const statuses = new Set(["enrolling", "verifying", "ready", "degraded", "offline", "revoked"]);
const strings = (value: unknown): value is string[] => Array.isArray(value) && value.every((item) => typeof item === "string");

function parseNode(value: unknown): ComputeNode {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("Invalid node inventory response.");
  const node = value as Record<string, unknown>;
  if (typeof node.id !== "string" || typeof node.name !== "string" ||
      (node.provider !== "incus" && node.provider !== "proxmox") ||
      typeof node.status !== "string" || !statuses.has(node.status) ||
      typeof node.schedulable !== "boolean" || !strings(node.capabilities) ||
      (node.last_seen_at !== undefined && node.last_seen_at !== null && typeof node.last_seen_at !== "string") ||
      (node.readiness_reasons !== undefined && !strings(node.readiness_reasons))) {
    throw new Error("Invalid node inventory response.");
  }
  return node as ComputeNode;
}

export async function loadNodes(apiBaseUrl: string, token: string, signal?: AbortSignal): Promise<ComputeNode[]> {
  const response = await fetch(`${apiBaseUrl.replace(/\/$/, "")}/operator/compute/nodes`, {
    headers: { Accept: "application/json", Authorization: `Bearer ${token.trim()}` },
    cache: "no-store",
    signal
  });
  if (!response.ok) throw new Error(`Infrastructure inventory is unavailable (HTTP ${response.status}).`);
  const data: unknown = await response.json();
  if (!data || typeof data !== "object" || !Array.isArray((data as { items?: unknown }).items)) {
    throw new Error("Invalid node inventory response.");
  }
  return (data as { items: unknown[] }).items.map(parseNode);
}
