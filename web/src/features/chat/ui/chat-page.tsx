import { useEffect } from "react";
import { chatUI } from "../../../lib/chat/chat-store";
import { useParams } from "../../../lib/router/router";
import { PageHeader } from "../../../template/page-header";
import { ChatSurface } from "./chat-surface";

/** /chat and /chat/:sessionId: the same ChatSurface as the overlay, with the route's extras. */
export function ChatPage() {
  const { sessionId: param } = useParams();
  useEffect(() => { if (param) chatUI.select(param); }, [param]);
  return (
    <>
      <PageHeader eyebrow="AI" title="Automaton" description="Provider-agnostic chat: your own Claude or ChatGPT subscription, running in your workspace. Same conversations as the chat panel." />
      <div className="h-[calc(100vh-220px)] min-h-[420px] overflow-hidden rounded-[var(--radius-card)] border border-line bg-surface">
        <ChatSurface variant="page" sessionId={param ?? null} />
      </div>
    </>
  );
}
