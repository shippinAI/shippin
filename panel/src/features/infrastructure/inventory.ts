export type ComputeNode = {
  id: string;
  name: string;
  provider: "incus" | "proxmox" | null;
  observed_hostname?: { value: string; source: "node-agent" | "control-plane" | "provider"; observed_at: string };
  nickname?: string;
  aliases?: string[];
  tags?: Record<string, string>;
  status: "enrolling" | "verifying" | "ready" | "degraded" | "offline" | "revoked";
  schedulable: boolean;
  capabilities: string[];
  last_seen_at?: string | null;
  readiness_reasons?: string[];
};

const statuses = new Set(["enrolling", "verifying", "ready", "degraded", "offline", "revoked"]);
const strings = (value: unknown): value is string[] => Array.isArray(value) && value.every((item) => typeof item === "string");
const label = (value: unknown): value is string => typeof value === "string" && value.trim().length > 0 && value.length <= 120;
const labels = (value: unknown): value is Record<string, string> => !!value && typeof value === "object" && !Array.isArray(value) && Object.values(value).every((item) => typeof item === "string" && item.length <= 256);
const timestamp = (value: unknown): value is string => typeof value === "string" && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/i.test(value) && !Number.isNaN(Date.parse(value));

function hostname(value: unknown): value is NonNullable<ComputeNode["observed_hostname"]> {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const item = value as Record<string, unknown>;
  return Object.keys(item).every((key) => ["value", "source", "observed_at"].includes(key)) &&
    typeof item.value === "string" && item.value.trim().length > 0 && item.value.length <= 253 &&
    (item.source === "node-agent" || item.source === "control-plane" || item.source === "provider") && timestamp(item.observed_at);
}

export function nodeDisplayName(node: ComputeNode): string {
  return node.nickname || node.observed_hostname?.value || node.name;
}

function parseNode(value: unknown): ComputeNode {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("Invalid node inventory response.");
  const node = value as Record<string, unknown>;
  if (typeof node.id !== "string" || typeof node.name !== "string" ||
      (node.provider !== "incus" && node.provider !== "proxmox" && node.provider !== null) ||
      (node.observed_hostname !== undefined && !hostname(node.observed_hostname)) ||
      (node.nickname !== undefined && !label(node.nickname)) ||
      (node.aliases !== undefined && (!strings(node.aliases) || !node.aliases.every(label) || new Set(node.aliases).size !== node.aliases.length)) ||
      (node.tags !== undefined && !labels(node.tags)) ||
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
