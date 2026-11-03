import type { Job } from "./job";
import type { AIProvider } from "./subscription-types";

/** M4 prompt → project (implementation-plan-3). No prompt text comes back except a short preview. */
export type Stack = { template: string; label: string; keywords: string[] };

export type PromptOptions = {
  enabled: boolean;
  max_bytes: number;
  stacks: Stack[];
  default_template: string;
  agents: AIProvider[];
  terms_note: string;
};

export type RunState = "queued" | "provisioning" | "waiting_ai" | "starting" | "running" | "exited" | "failed";

export type AgentRun = {
  id: string;
  project_id: string;
  workspace_id?: string;
  provider: string;
  template: string;
  prompt_preview: string;
  state: RunState;
  error?: string;
  created_at: string;
};

/** The agent's live state from its devspace: booleans, a timestamp, a count (never pane content). */
export type AgentLive = { session: boolean; activity: number; exited: boolean; exit_code: number; files_changed: number };

export type AgentView = {
  run: AgentRun | null;
  live?: AgentLive;
  term_url?: string;
  provider_label: string;
  connected: boolean;
  mine: boolean;
};

export type PromptCreated = { project_id: string; name: string; template: string; run: AgentRun; job?: Job };
