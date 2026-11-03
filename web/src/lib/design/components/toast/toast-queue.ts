import { UNSTABLE_ToastQueue as ToastQueue } from "react-aria-components";

export type ToastTone = "success" | "error" | "info";
export type ToastContent = { title: string; description?: string; tone: ToastTone };

/** App-wide toast queue: at most 3 visible (CONSOLE-IA toast rules). */
export const toastQueue = new ToastQueue<ToastContent>({ maxVisibleToasts: 3 });

export const toast = {
  success: (title: string, description?: string) =>
    toastQueue.add({ title, description, tone: "success" }, { timeout: 4000 }),
  info: (title: string, description?: string) =>
    toastQueue.add({ title, description, tone: "info" }, { timeout: 4000 }),
  /** Errors persist until dismissed. */
  error: (title: string, description?: string) => toastQueue.add({ title, description, tone: "error" }),
};
