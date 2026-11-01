import { AlertTriangle, Bot, User } from "lucide-react";
import type { A2UIBlock, ChatMessage } from "../../../lib/chat/chat-types";
import { A2UIRenderer } from "../a2ui/a2ui-renderer";

type Props = { m: Pick<ChatMessage, "role" | "content" | "ui">; onPrompt: (t: string) => void; onCopy?: (b: A2UIBlock) => void; fresh?: boolean; streaming?: boolean };

/** One chat message: text plus any A2UI blocks the agent (or server) attached. */
export function MessageItem({ m, onPrompt, onCopy, fresh, streaming }: Props) {
  const Icon = m.role === "user" ? User : m.role === "error" ? AlertTriangle : Bot;
  const tint = m.role === "error" ? "text-warning" : m.role === "user" ? "text-fg-muted" : "text-primary";
  return (
    <li className="flex gap-3">
      <Icon className={`mt-1 size-4 shrink-0 ${tint}`} aria-hidden />
      <div className="min-w-0 flex-1">
        <p className="sr-only">{m.role === "user" ? "You" : m.role === "error" ? "Problem" : "Assistant"}</p>
        {m.content && <p className="whitespace-pre-wrap break-words text-sm text-fg">{m.content}{streaming && <span className="animate-pulse motion-reduce:animate-none">▍</span>}</p>}
        {!!m.ui?.length && <A2UIRenderer blocks={m.ui} onPrompt={onPrompt} onCopy={onCopy} fresh={fresh} />}
      </div>
    </li>
  );
}
