import { useMemo } from "react";
import type { ChatContext } from "../../../lib/chat/chat-context";
import { contextMessages } from "../../../lib/chat/context-a2ui";
import { navigate } from "../../../lib/router/location";
import { A2UISurface } from "../../a2ui/a2ui-surface";
import { useSurfaces } from "../../a2ui/use-surfaces";

type Props = { ctx: ChatContext; onPrompt?: (text: string) => void };

/** Route-specific UI inside ChatSurface, sent as an A2UI v0.9 surface from Clyffy: a context
 * card plus one-tap prompts (prompts need an open session). */
export function ChatContextSlot({ ctx, onPrompt }: Props) {
  const messages = useMemo(() => contextMessages(ctx, !!onPrompt), [ctx, onPrompt]);
  const { surfaces, edit } = useSurfaces(messages);
  const s = surfaces[`chat-context-${ctx.kind}`];
  if (!s) return null;
  return (
    <div className="border-b border-line p-4">
      <A2UISurface surface={s} label="Clyffy suggestions" onData={(p, v) => edit(s.id, p, v)}
        onAction={(a) => {
          if (a.name === "prompt" && onPrompt) onPrompt(String(a.context.text));
          if (a.name === "go") navigate(String(a.context.to));
        }} />
    </div>
  );
}
