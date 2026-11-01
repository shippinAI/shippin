import { useEffect } from "react";
import { Lightbulb } from "lucide-react";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import type { A2UIBlock } from "../../../lib/chat/chat-types";
import { A2UIAction } from "./a2ui-action";
import { A2UIForm } from "./a2ui-form";
import { A2UIModal } from "./a2ui-modal";

type Props = { blocks: A2UIBlock[]; onPrompt: (text: string) => void; onCopy?: (b: A2UIBlock) => void; fresh?: boolean };

/** Renders agent UI from the whitelisted catalog only; anything else was dropped server-side and
 * is ignored here too. Toasts fire once, for fresh messages only. */
export function A2UIRenderer({ blocks, onPrompt, onCopy, fresh }: Props) {
  useEffect(() => {
    if (!fresh) return;
    for (const b of blocks) if (b.type === "toast" && b.title) toast[b.tone ?? "info"](b.title, b.body);
  }, [blocks, fresh]);
  return (
    <div className="mt-2 space-y-2">
      {blocks.map((b, i) => {
        switch (b.type) {
          case "card":
            return (
              <div key={i} className="rounded-[var(--radius-card)] border border-line bg-canvas/40 p-3">
                {b.title && <p className="text-sm font-medium text-fg">{b.title}</p>}
                {b.body && <p className="mt-1 whitespace-pre-line text-sm text-fg-muted">{b.body}</p>}
                {!!b.items?.length && <div className="mt-3 flex flex-wrap gap-2">{b.items.map((it, j) => <A2UIAction key={j} b={it} onPrompt={onPrompt} onCopy={onCopy} />)}</div>}
              </div>
            );
          case "tip":
            return (
              <div key={i} role="note" className="flex gap-2 rounded-[var(--radius-card)] border border-primary/25 bg-primary/5 p-3 text-sm">
                <Lightbulb className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden />
                <div><p className="font-medium text-fg">{b.title}</p>{b.body && <p className="text-fg-muted">{b.body}</p>}</div>
              </div>
            );
          case "button":
            return <A2UIAction key={i} b={b} onPrompt={onPrompt} onCopy={onCopy} />;
          case "form":
            return <A2UIForm key={i} b={b} onPrompt={onPrompt} />;
          case "modal":
            return <A2UIModal key={i} b={b} onPrompt={onPrompt} onCopy={onCopy} />;
          default:
            return null;
        }
      })}
    </div>
  );
}
