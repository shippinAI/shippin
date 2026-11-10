import { useState } from "react";
import { Form, TextArea, TextField } from "react-aria-components";
import { Send } from "lucide-react";
import { IconButton } from "../../../lib/design/components/button/icon-button";

/** Message box: Enter sends, Shift+Enter adds a line. */
export function Composer({ onSend, isDisabled }: { onSend: (text: string) => void; isDisabled?: boolean }) {
  const [text, setText] = useState("");
  const send = () => {
    const t = text.trim();
    if (!t || isDisabled) return;
    onSend(t);
    setText("");
  };
  return (
    <Form className="flex items-end gap-2 border-t border-line p-3" onSubmit={(e) => { e.preventDefault(); send(); }}>
      <TextField aria-label="Message" value={text} onChange={setText} className="flex-1">
        <TextArea rows={2} placeholder="Ask your AI… (Enter to send)"
          onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); send(); } }}
          className="w-full resize-none rounded-lg border border-line-control bg-canvas/60 px-3 py-2 text-base text-fg outline-none data-focused:border-primary" />
      </TextField>
      <IconButton icon={Send} label="Send message" type="submit" isDisabled={isDisabled || !text.trim()} />
    </Form>
  );
}
