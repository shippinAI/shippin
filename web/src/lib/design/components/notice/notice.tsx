import { CircleCheck, CircleX, Info, TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "../cn/cn";

export type NoticeTone = "info" | "success" | "warning" | "danger";

const tones: Record<NoticeTone, { box: string; icon: string; Icon: typeof Info }> = {
  info: { box: "border-info/30 bg-info/10", icon: "text-info", Icon: Info },
  success: { box: "border-success/30 bg-success/10", icon: "text-success", Icon: CircleCheck },
  warning: { box: "border-warning/30 bg-warning/10", icon: "text-warning", Icon: TriangleAlert },
  danger: { box: "border-danger/35 bg-danger/10", icon: "text-danger", Icon: CircleX },
};

type Props = { tone?: NoticeTone; title?: ReactNode; children?: ReactNode; actions?: ReactNode; className?: string };

/** A boxed message on a page: an icon and words, so the tone is never the only signal. Danger is an
 * alert that screen readers announce at once; the others are a polite status. */
export function Notice({ tone = "info", title, children, actions, className }: Props) {
  const t = tones[tone];
  return (
    <div role={tone === "danger" ? "alert" : "status"} className={cn("flex gap-3 rounded-xl border px-4 py-3 text-sm", t.box, className)}>
      <t.Icon aria-hidden className={cn("mt-0.5 size-4 shrink-0", t.icon)} />
      <div className="min-w-0 flex-1 space-y-1">
        {title && <p className="font-medium text-fg">{title}</p>}
        {children && <div className="text-fg-muted">{children}</div>}
        {actions && <div className="flex flex-wrap items-center gap-2 pt-1.5">{actions}</div>}
      </div>
    </div>
  );
}
