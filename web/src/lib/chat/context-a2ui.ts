import { A2UI_VERSION as v, SHIPPIN_CATALOG, type A2UIComponent, type A2UIMessage } from "../a2ui/a2ui-types";
import type { ChatContext } from "./chat-context";

const intro: Record<ChatContext["kind"], string> = {
  terminal: "You're in the workspace terminal (Zellij session \"shippin\"). Ask about anything on screen.",
  connect: "Connect GitHub and your AI CLIs once; the workspace keeps them signed in.",
  publish: "Publishing snapshots this devspace and serves it at a public HTTPS address.",
  workspace: "Ask about this workspace: what's running, ports, previews.",
  chat: "",
  general: "Ask anything about this panel.",
};

/** The assistant's per-route card as A2UI messages. No card on /chat itself (the rail is the extra). */
export function contextMessages(ctx: ChatContext, canPrompt: boolean): A2UIMessage[] {
  if (ctx.kind === "chat") return [];
  const s = `chat-context-${ctx.kind}`;
  const comps: A2UIComponent[] = [
    { id: "root", component: "Card", child: "col" },
    { id: "head", component: "Text", text: "Assistant", variant: "caption" },
    { id: "intro", component: "Text", text: intro[ctx.kind] },
  ];
  const kids = ["head", "intro"];
  if (canPrompt && ctx.prompts.length) {
    kids.push("prompts");
    comps.push({ id: "prompts", component: "Row", children: { path: "/prompts", componentId: "p" } },
      { id: "p-label", component: "Text", text: { path: "text" } },
      { id: "p", component: "Button", child: "p-label", tooltip: "Send this to the assistant", action: { event: { name: "prompt", context: { text: { path: "text" } } } } });
  }
  if (ctx.kind === "terminal" && ctx.tenantId) {
    kids.push("connect");
    comps.push({ id: "connect-label", component: "Text", text: "Connect accounts" },
      { id: "connect", component: "Button", child: "connect-label", variant: "borderless", tooltip: "Sign GitHub and the AI CLIs in",
        action: { event: { name: "go", context: { to: `/t/${ctx.tenantId}/workspaces/${ctx.workspaceId}/connect` } } } });
  }
  comps.push({ id: "col", component: "Column", children: kids });
  return [
    { version: v, createSurface: { surfaceId: s, catalogId: SHIPPIN_CATALOG } },
    { version: v, updateComponents: { surfaceId: s, components: comps } },
    { version: v, updateDataModel: { surfaceId: s, path: "/prompts", value: ctx.prompts.map((text) => ({ text })) } },
  ];
}
