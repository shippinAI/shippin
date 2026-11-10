import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { IconButton } from "../button/icon-button";
import { toast } from "../toast/toast-queue";

type Props = { label: string; code: string };

/** Monospace snippet with a copy action and toast. */
export function CodeBlock({ label, code }: Props) {
  const [done, setDone] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setDone(true);
      toast.success(`${label} copied`);
      setTimeout(() => setDone(false), 1500);
    } catch {
      toast.error("Copy failed", "Your browser blocked clipboard access.");
    }
  };
  return (
    <div>
      <div className="mb-1.5 text-xs font-medium text-fg-muted">{label}</div>
      <div className="group relative rounded-lg border border-line bg-canvas/80">
        <pre className="overflow-x-auto px-3.5 py-3 pr-12 font-mono text-sm leading-relaxed text-fg">{code}</pre>
        <div className="absolute top-1.5 right-1.5">
          <IconButton icon={done ? Check : Copy} label={`Copy ${label.toLowerCase()}`} onPress={copy} />
        </div>
      </div>
    </div>
  );
}
