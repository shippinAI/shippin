/** A2UI block, as validated server-side against the whitelisted catalog. */
export type A2UIBlock = {
  type: "card" | "button" | "form" | "modal" | "toast" | "tip";
  /** Stable id for a server-built action (an app id). */
  name?: string;
  title?: string;
  body?: string;
  label?: string;
  prompt?: string;
  href?: string;
  /** Exact text a Copy button puts on the clipboard. Only official install commands survive the server. */
  copy?: string;
  tone?: "success" | "error" | "info";
  fields?: { name: string; label: string; kind: "text" | "textarea" | "select"; options?: string[] }[];
  items?: A2UIBlock[];
};

export type ChatMessage = { id: string; role: "user" | "assistant" | "error"; content: string; ui: A2UIBlock[]; created_at: string };

export type ChatSession = {
  id: string;
  tenant_id: string;
  project_id?: string;
  workspace_id: string;
  provider: string;
  title: string;
  created_at: string;
  updated_at: string;
};

export type ChatProvider = { id: string; name: string; subscription: string };

export type ChatReady = { ready: boolean; detail: string; ui?: A2UIBlock[]; provider: string; workspace: string };
