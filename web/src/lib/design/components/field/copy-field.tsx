import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { IconButton } from "../button/icon-button";
import { toast } from "../toast/toast-queue";

type Props = { label: string; value: string; mono?: boolean; hint?: string };

/** Read-only setting row with a copy action. */
export function CopyField({ label, value, mono, hint }: Props) {
  const [done, setDone] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setDone(true);
      toast.success(`${label} copied`);
      setTimeout(() => setDone(false), 1500);
    } catch {
      toast.error("Copy failed", "Your browser blocked clipboard access.");
    }
  };
  return (
    <div className="grid grid-cols-[200px_1fr] items-center gap-6 border-b border-line px-5 py-3.5 last:border-b-0">
      <div>
        <div className="text-[13px] font-medium text-fg">{label}</div>
        {hint && <div className="mt-0.5 text-xs text-fg-subtle">{hint}</div>}
      </div>
      <div className="flex min-w-0 items-center gap-2 rounded-[var(--radius-control)] border border-line bg-canvas/60 py-1 pr-1 pl-3">
        <span className={`min-w-0 flex-1 truncate text-[13px] text-fg-muted ${mono ? "font-mono" : ""}`}>{value || "—"}</span>
        <IconButton icon={done ? Check : Copy} label={`Copy ${label.toLowerCase()}`} onPress={copy} isDisabled={!value} />
      </div>
    </div>
  );
}
