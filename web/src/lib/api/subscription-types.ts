import type { Job } from "./job";

/** M2: per-user AI subscription status. There is deliberately no credential field anywhere. */
export type AIProvider = { id: string; label: string; vendor?: string; login_command: string; device_url?: string };

export type ConnectionStatus = "not_connected" | "activating" | "connected" | "needs_reauth" | "limited" | "disconnected";

export type AIHome = {
  id: string;
  tenant_id: string;
  account_id: string;
  volume: string;
  state: "creating" | "ready" | "error";
  error?: string;
  utility_host?: string;
  utility_state: "stopped" | "starting" | "running" | "error";
};

export type AIConnection = { provider: string; home_id: string; tenant_id: string; status: ConnectionStatus; plan?: string; checked_at?: string };

export type MySubscriptions = {
  enabled: boolean;
  terms_note: string;
  touch_seconds: number;
  providers: AIProvider[];
  homes: AIHome[];
  connections: AIConnection[];
};

export type Activation = { home: AIHome; provider: string; terminal_url: string; login_command: string; job?: Job };
