import type { Job } from "./job";

/** Managed service instance (07-multi-tenancy) and rrflow/state views (02c-rrflow "Modules"). */
export type ServiceBinding = { id: string; workspace_id: string; state: string; error?: string };

export type ServiceInstance = {
  id: string;
  project_id: string;
  kind: "rrflow-state";
  state: "pending" | "provisioning" | "ready" | "error" | string;
  error?: string;
  created_at: string;
  bindings: ServiceBinding[];
};

export type ServiceList = { rrflow_enabled: boolean; items: ServiceInstance[] };

export type AttachResult = { instance: ServiceInstance; job?: Job };

export type RRSession = { id: string; agent: string; mode: string; status: string; started_at: string; ended_at?: string };

export type RRMemory = { scope: string; key: string; value: string; version: number; updated_at: string };

export type RRDecision = {
  seq: number;
  id: string;
  actor: string;
  action: string;
  resource: string;
  effect: string;
  reason?: string;
  at: string;
  hash: string;
};

export type RRKind = "sessions" | "memory" | "decisions";
