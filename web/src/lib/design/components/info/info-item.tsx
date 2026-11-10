import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { Button } from "react-aria-components";
import { Tooltip } from "../tooltip/tooltip";
import { toast } from "../toast/toast-queue";

type Props = { label: string; value: string; mono?: boolean; copy?: boolean };

/** Compact label/value pair with click-to-copy, for resource summary strips. */
export function InfoItem({ label, value, mono, copy = true }: Props) {
  const [done, setDone] = useState(false);
  const onCopy = async () => {
    await navigator.clipboard.writeText(value).catch(() => undefined);
    setDone(true);
    toast.success(`${label} copied`);
    setTimeout(() => setDone(false), 1200);
  };
  return (
    <div className="min-w-0">
      <div className="text-xs font-medium tracking-wide text-fg-subtle uppercase">{label}</div>
      <div className="mt-1 flex items-center gap-1.5">
        <span className={`truncate text-base text-fg ${mono ? "font-mono text-sm" : ""}`}>{value || "—"}</span>
        {copy && value && (
          <Tooltip content={`Copy ${label.toLowerCase()}`}>
            <Button aria-label={`Copy ${label}`} onPress={onCopy} className="focus-ring cursor-default rounded-sm p-0.5 text-fg-subtle data-hovered:text-fg">
              {done ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
            </Button>
          </Tooltip>
        )}
      </div>
    </div>
  );
}
