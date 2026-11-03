// Server registry (/v1/tenants/{id}/servers; spec schemas/servers.yaml). No hash is ever sent.
export type ServerState = "planned" | "pending" | "active" | "stale" | "revoked";

export type Server = {
  id: string; slug: string; label: string; site: string; state: ServerState; estate: boolean;
  hostname: string; agent_version: string; last_seen_at: string | null; enrolled_at: string | null; created_at: string;
};

export type ServerList = { items: Server[]; heartbeat_seconds: number; missed_beats: number };

/** Shown once; the console keeps only its hash. */
export type EnrollToken = { token: string; expires_at: string };
