import { useEffect, useRef } from "react";
import type { Workspace } from "../api/types";
import { toast } from "../design/components/toast/toast-queue";

/** Emits a toast when a workspace settles into running, stopped or failed. */
export function useStateToasts(items?: Workspace[]) {
  const prev = useRef<Map<string, string>>(new Map());
  useEffect(() => {
    if (!items) return;
    for (const w of items) {
      const before = prev.current.get(w.id);
      if (before && before !== w.state) {
        if (w.state === "running") toast.success(`${w.name} is running`, before === "provisioning" ? "Open it in your browser." : undefined);
        if (w.state === "stopped") toast.success(`${w.name} stopped`);
        if (w.state === "failed") toast.error(`${w.name} needs attention`, w.error);
      }
      prev.current.set(w.id, w.state);
    }
  }, [items]);
}
