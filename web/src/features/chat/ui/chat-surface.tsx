import { Maximize2, Minimize2, Plus } from "lucide-react";
import { chatUI, useChatUI } from "../../../lib/chat/chat-store";
import { chatContextFor, type ChatVariant } from "../../../lib/chat/chat-context";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { navigate } from "../../../lib/router/location";
import { useLocation } from "../../../lib/router/router";
import { ChatContextSlot } from "./chat-context-slot";
import { ChatThread } from "./chat-thread";
import { NewChatForm } from "./new-chat-form";
import { SessionList } from "./session-list";

/** `sessionId` overrides the store (the /chat route owns its id in the URL). */
type Props = { variant: ChatVariant; sessionId?: string | null };

/** THE chat: the /chat route and the overlay both render this. The route decides the extras:
 * a session rail on /chat, page-specific prompts and an inline surface elsewhere. */
export function ChatSurface({ variant, sessionId: routeId }: Props) {
  const { pathname } = useLocation();
  const store = useChatUI().sessionId;
  const sessionId = routeId === undefined ? store : routeId;
  const ctx = chatContextFor(pathname, variant);
  const open = (id: string | null) => {
    chatUI.select(id);
    if (variant === "page") navigate(id ? `/chat/${id}` : "/chat");
  };
  const swap = () => {
    if (ctx.swap === "expand") { chatUI.closeOverlay(); navigate(sessionId ? `/chat/${sessionId}` : "/chat"); return; }
    if (window.history.length > 1) window.history.back(); else navigate("/");
    chatUI.openOverlay(sessionId);
  };
  const body = sessionId ? (
    <ChatThread key={sessionId} sessionId={sessionId} slot={(send) => <ChatContextSlot ctx={ctx} onPrompt={send} />} />
  ) : (
    <div><ChatContextSlot ctx={ctx} /><NewChatForm onCreated={open} /></div>
  );
  return (
    <div data-chat-surface={variant} data-chat-context={ctx.kind}
      className={ctx.sessions ? "grid h-full min-h-0 grid-cols-[240px_1fr]" : "flex h-full min-h-0 flex-col"}>
      {ctx.sessions && (
        <aside className="overflow-y-auto border-r border-line" aria-label="Conversations">
          <div className="flex justify-end p-2"><IconButton icon={Plus} label="New chat" onPress={() => open(null)} /></div>
          <SessionList selected={sessionId} onSelect={open} />
        </aside>
      )}
      <section className="flex min-h-0 flex-1 flex-col">
        <div className="flex items-center gap-1 border-b border-line px-2 py-1">
          <span className="flex-1 truncate px-2 text-[12px] text-fg-subtle">{ctx.label ? `Context: ${ctx.label}` : ""}</span>
          {!ctx.sessions && <IconButton icon={Plus} label="New chat" onPress={() => open(null)} />}
          <IconButton icon={ctx.swap === "expand" ? Maximize2 : Minimize2} label={ctx.swap === "expand" ? "Open full page" : "Pop into overlay"} onPress={swap} />
        </div>
        <div className="min-h-0 flex-1">{body}</div>
      </section>
    </div>
  );
}
