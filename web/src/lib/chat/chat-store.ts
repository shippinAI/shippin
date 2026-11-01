import { useSyncExternalStore } from "react";

/** One UI store for the overlay and /chat: which session is active and whether the overlay is open.
 * The conversation itself lives on the server, so both surfaces always show the same rows. */
type State = { open: boolean; sessionId: string | null };

const key = "shippin.chat.session";
let state: State = { open: false, sessionId: localStorage.getItem(key) };
const listeners = new Set<() => void>();

function set(next: Partial<State>) {
  state = { ...state, ...next };
  if (next.sessionId !== undefined) {
    if (next.sessionId) localStorage.setItem(key, next.sessionId);
    else localStorage.removeItem(key);
  }
  listeners.forEach((l) => l());
}

export const chatUI = {
  openOverlay: (sessionId?: string | null) => set(sessionId === undefined ? { open: true } : { open: true, sessionId }),
  closeOverlay: () => set({ open: false }),
  setOpen: (open: boolean) => set({ open }),
  select: (sessionId: string | null) => set({ sessionId }),
};

const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => listeners.delete(l);
};

export const useChatUI = (): State => useSyncExternalStore(subscribe, () => state);
