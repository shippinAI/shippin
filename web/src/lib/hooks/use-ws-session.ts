import { useCallback, useEffect, useState } from "react";

export type WsSession = "checking" | "ok" | "signin";

const RECHECK_MS = 5 * 60 * 1000;

/**
 * Whether the browser holds a valid sign-in cookie for a workspace host. The console and the
 * IdP refuse to be framed, so the sign-in hop runs in a pop-up (the workspace host's
 * /_shippin/popup), never inside the IDE frame. Re-checked on focus and every few minutes, so
 * an expired cookie shows the sign-in prompt instead of a broken frame.
 */
export function useWsSession(url: string | undefined) {
  const origin = url ? new URL(url).origin : "";
  const [state, setState] = useState<WsSession>("checking");

  const check = useCallback(async () => {
    if (!origin) return;
    try {
      const r = await fetch(`${origin}/_shippin/status`, { credentials: "include", cache: "no-store" });
      setState(r.status === 204 ? "ok" : "signin");
    } catch {
      setState("signin");
    }
  }, [origin]);

  useEffect(() => {
    if (!origin) return;
    void check();
    const onMsg = (e: MessageEvent) => {
      if (e.origin === origin && (e.data as { type?: string } | null)?.type === "shippin-ws-ready") void check();
    };
    const onFocus = () => void check();
    window.addEventListener("message", onMsg);
    window.addEventListener("focus", onFocus);
    const t = window.setInterval(() => void check(), RECHECK_MS);
    return () => {
      window.removeEventListener("message", onMsg);
      window.removeEventListener("focus", onFocus);
      window.clearInterval(t);
    };
  }, [origin, check]);

  /** Must run from a user gesture (pop-up blockers). */
  const signIn = useCallback(() => {
    if (origin) window.open(`${origin}/_shippin/popup`, "shippin-ws-signin", "popup,width=520,height=680");
  }, [origin]);

  return { state, signIn };
}
