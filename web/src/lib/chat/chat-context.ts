/**
 * What the one ChatSurface adds for where it is shown. `variant` is the host (the /chat page or
 * the overlay); `pathname` is the route underneath. Pure so the routing rules are unit-tested.
 */
export type ChatVariant = "page" | "overlay";

export type ChatContext = {
  kind: "terminal" | "connect" | "publish" | "workspace" | "chat" | "general";
  /** Session list rail (full page only). */
  sessions: boolean;
  /** Expand to /chat (overlay) or pop back into the overlay (page). */
  swap: "expand" | "pop";
  tenantId?: string;
  workspaceId?: string;
  /** Route-specific one-tap prompts shown above the thread. */
  prompts: string[];
  /** Short label for the context chip, e.g. "Terminal · ws-1". */
  label?: string;
};

const ws = /^\/t\/([^/]+)\/workspaces\/([^/]+)(?:\/([^/]+))?/;

export function chatContextFor(pathname: string, variant: ChatVariant): ChatContext {
  const base = { sessions: variant === "page", swap: variant === "page" ? "pop" : "expand" } as const;
  if (pathname.startsWith("/chat")) return { ...base, kind: "chat", prompts: [] };
  const m = ws.exec(pathname);
  if (!m) return { ...base, kind: "general", prompts: ["What can I do in Shippin?", "How do I publish a project?"] };
  const [, tenantId, workspaceId, sub] = m;
  const at = { tenantId, workspaceId };
  if (sub === "terminal")
    return { ...base, ...at, kind: "terminal", label: "Terminal", prompts: ["Explain the last command I ran", "How do I split panes in Zellij?", "Open this workspace in WezTerm"] };
  if (sub === "connect")
    return { ...base, ...at, kind: "connect", label: "Connect accounts", prompts: ["Which accounts should I connect?", "Browser sign-in or token?"] };
  if (sub === "publish" || pathname.endsWith("/publish"))
    return { ...base, ...at, kind: "publish", label: "Publish", prompts: ["Is my app ready to publish?", "How do I roll back a release?"] };
  return { ...base, ...at, kind: "workspace", label: "Workspace", prompts: ["What's running in this workspace?"] };
}
