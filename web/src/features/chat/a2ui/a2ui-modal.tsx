import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import type { A2UIBlock } from "../../../lib/chat/chat-types";
import { A2UIAction } from "./a2ui-action";

/** An A2UI modal: a trigger button in the message; the dialog holds the body and actions. */
export function A2UIModal({ b, onPrompt, onCopy }: { b: A2UIBlock; onPrompt: (text: string) => void; onCopy?: (b: A2UIBlock) => void }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button size="sm" variant="secondary" tooltip={b.title ?? "Details"} onPress={() => setOpen(true)}>
        {b.label || b.title || "Details"}
      </Button>
      <ModalDialog isOpen={open} onOpenChange={setOpen} title={b.title ?? ""}>
        <div className="px-6 py-5">
        <p className="whitespace-pre-line text-sm text-fg-muted">{b.body}</p>
        {!!b.items?.length && (
          <div className="mt-4 flex flex-wrap gap-2">
            {b.items.map((it, i) => (
              <A2UIAction key={i} b={it} onPrompt={(t) => { setOpen(false); onPrompt(t); }} onCopy={onCopy} />
            ))}
          </div>
        )}
        </div>
      </ModalDialog>
    </>
  );
}
