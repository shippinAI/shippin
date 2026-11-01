import { CheckCircle2, Info, X, XCircle } from "lucide-react";
import {
  Button,
  Text,
  UNSTABLE_Toast as Toast,
  UNSTABLE_ToastContent as ToastContent,
  UNSTABLE_ToastRegion as ToastRegion,
} from "react-aria-components";
import { toastQueue, type ToastTone } from "./toast-queue";

const icons: Record<ToastTone, typeof Info> = { success: CheckCircle2, error: XCircle, info: Info };
const tones: Record<ToastTone, string> = { success: "text-success", error: "text-danger", info: "text-info" };

/** Bottom-right glass toast stack. */
export function ToastRegionHost() {
  return (
    <ToastRegion queue={toastQueue} className="fixed right-5 bottom-5 z-[60] flex w-[360px] flex-col-reverse gap-2 outline-none">
      {({ toast }) => {
        const Icon = icons[toast.content.tone];
        return (
          <Toast
            toast={toast}
            className="glass focus-ring flex items-start gap-3 rounded-xl border border-line-strong p-3.5 shadow-[var(--shadow-e3)]"
          >
            <Icon className={`mt-0.5 size-4.5 shrink-0 ${tones[toast.content.tone]}`} strokeWidth={2} />
            <ToastContent className="min-w-0 flex-1">
              <Text slot="title" className="block text-sm font-medium text-fg">{toast.content.title}</Text>
              {toast.content.description && (
                <Text slot="description" className="mt-0.5 block text-[13px] text-fg-muted">{toast.content.description}</Text>
              )}
            </ToastContent>
            <Button slot="close" aria-label="Dismiss" className="focus-ring rounded p-0.5 text-fg-subtle data-hovered:text-fg">
              <X className="size-4" />
            </Button>
          </Toast>
        );
      }}
    </ToastRegion>
  );
}
