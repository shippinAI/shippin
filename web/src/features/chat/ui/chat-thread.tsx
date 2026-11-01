import { useEffect, useRef, type ReactNode } from "react";
import { useChatReady, useChatSession, useSendChat } from "../../../lib/chat/use-chat";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { useCopyApp } from "../../../lib/hooks/use-apps";
import { AppCards } from "../../apps/ui/app-cards";
import { Composer } from "./composer";
import { MessageItem } from "./message-item";
import { TipsPanel } from "./tips-panel";

/** A server-backed conversation; the overlay and /chat both render this for the same session id. */
export function ChatThread({ sessionId, slot }: { sessionId: string; slot?: (send: (text: string) => void) => ReactNode }) {
  const q = useChatSession(sessionId);
  const ready = useChatReady(sessionId);
  const { send, draft, pending } = useSendChat(sessionId);
  const end = useRef<HTMLDivElement>(null);
  const msgs = q.data?.messages ?? [];
  // Braces: newer Chromium returns a Promise from scrollIntoView, which React would call as cleanup.
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [msgs.length, draft]);
  const tenantId = q.data?.session.tenant_id ?? "";
  const copy = useCopyApp(tenantId);
  const onPrompt = (t: string) => send.mutate(t);
  const onCopy = (b: { name?: string }) => {
    if (b.name) copy.mutate(b.name);
  };
  const lastId = msgs[msgs.length - 1]?.id;
  if (q.isPending) return <Skeleton className="m-4 h-40" />;
  if (q.isError) return <p role="alert" className="p-4 text-sm text-danger">This chat couldn't be loaded.</p>;
  return (
    <div className="flex h-full min-h-0 flex-col">
      {slot?.(onPrompt)}
      <div className="flex-1 overflow-y-auto p-4">
        <TipsPanel />
        {tenantId && (
          <details open className="mb-4 rounded-[var(--radius-card)] border border-line bg-canvas/40 p-3">
            <summary className="cursor-pointer text-sm font-medium text-fg">Install an app in your terminal</summary>
            <p className="mt-2 text-sm text-fg-muted">Copy a command and paste it into your own terminal. It installs the app and starts the vendor sign-in.</p>
            <div className="mt-3"><AppCards tenantId={tenantId} /></div>
          </details>
        )}
        {ready.data && !ready.data.ready && msgs.length === 0 && (
          <ul className="mb-4"><MessageItem m={{ role: "error", content: ready.data.detail, ui: ready.data.ui ?? [] }} onPrompt={onPrompt} onCopy={onCopy} /></ul>
        )}
        <ul className="space-y-4" aria-live="polite">
          {msgs.map((m) => <MessageItem key={m.id} m={m} onPrompt={onPrompt} onCopy={onCopy} fresh={m.id === lastId && send.isSuccess} />)}
          {pending && !msgs.some((m) => m.role === "user" && m.content === pending) && (
            <MessageItem m={{ role: "user", content: pending, ui: [] }} onPrompt={onPrompt} />
          )}
          {draft !== null && (
            <MessageItem m={{ role: "assistant", content: draft || "Thinking in your workspace…", ui: [] }} onPrompt={onPrompt} streaming />
          )}
        </ul>
        <div ref={end} />
      </div>
      <Composer onSend={(t) => send.mutate(t)} isDisabled={send.isPending} />
    </div>
  );
}
