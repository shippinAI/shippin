import { Button } from "../../../lib/design/components/button/button";
import { LinkButton } from "../../../lib/design/components/button/link-button";
import type { A2UIBlock } from "../../../lib/chat/chat-types";
import { CopyCommand, copyText } from "./copy-command";

type Props = { b: A2UIBlock; onPrompt: (text: string) => void; onCopy?: (b: A2UIBlock) => void };

/** An A2UI button: a copy command, a console link, an https link, or a prompt. */
export function A2UIAction({ b, onPrompt, onCopy }: Props) {
  if (b.copy) {
    return (
      <div className="flex w-full basis-full flex-col items-start gap-2">
        <CopyCommand command={b.copy} />
        <Button
          size="sm"
          variant="secondary"
          tooltip="Copy this command, then paste it in your terminal"
          onPress={() => {
            void copyText(b.copy!);
            onCopy?.(b);
          }}
        >
          {b.label || "Copy"}
        </Button>
      </div>
    );
  }
  if (b.href) {
    return (
      <LinkButton href={b.href} size="sm" variant="secondary" tooltip={b.href}>
        {b.label}
      </LinkButton>
    );
  }
  return (
    <Button size="sm" variant="secondary" tooltip="Send this to the assistant" onPress={() => b.prompt && onPrompt(b.prompt)}>
      {b.label}
    </Button>
  );
}
