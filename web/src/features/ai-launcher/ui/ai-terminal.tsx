import "@xterm/xterm/css/xterm.css";
import { useEffect, useRef, useState } from "react";
import { attachURL } from "../use-ai-sessions";

const cell = { w: 7.8, h: 17 };
type State = "connecting" | "open" | "reconnecting";

/** xterm.js attached to the CLI's tmux session. A dropped socket reconnects with backoff and
 *  re-attaches to the same session, so the CLI (and a login in progress) is never lost. */
export function AITerminal({ id }: { id: string }) {
  const host = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<State>("connecting");
  const [attaches, setAttaches] = useState(0);
  useEffect(() => {
    const el = host.current;
    if (!el) return;
    let gone = false, ws: WebSocket | null = null, timer = 0, tries = 0;
    let dispose = () => {};
    void import("@xterm/xterm").then(({ Terminal }) => {
      if (gone) return;
      const cols = Math.max(40, Math.floor((el.clientWidth - 16) / cell.w));
      const rows = Math.max(10, Math.floor((el.clientHeight - 16) / cell.h));
      const term = new Terminal({ cols, rows, cursorBlink: true, fontSize: 13, fontFamily: "var(--font-mono), monospace", theme: { background: "#0b0d12" } });
      term.open(el);
      const enc = new TextEncoder();
      const connect = () => {
        ws = new WebSocket(attachURL(window.location, id, term.rows, term.cols));
        ws.binaryType = "arraybuffer";
        ws.onopen = () => { tries = 0; setState("open"); setAttaches((n) => n + 1); term.focus(); };
        ws.onmessage = (e) => term.write(new Uint8Array(e.data as ArrayBuffer));
        ws.onclose = () => {
          if (gone) return;
          setState("reconnecting");
          timer = window.setTimeout(connect, Math.min(8000, 400 * 2 ** tries++));
        };
      };
      const sub = term.onData((d) => ws?.readyState === WebSocket.OPEN && ws.send(enc.encode(d)));
      connect();
      dispose = () => { sub.dispose(); term.dispose(); };
    });
    return () => { gone = true; window.clearTimeout(timer); ws?.close(); dispose(); };
  }, [id]);
  return (
    <div className="relative h-full min-h-0 rounded-xl bg-[#0b0d12]" data-attaches={attaches} data-state={state}>
      <div ref={host} className="absolute inset-0 p-2" aria-label={`${id} terminal`} role="region" />
      {state !== "open" && (
        <p className="absolute right-3 top-2 rounded bg-surface-2 px-2 py-0.5 text-xs text-fg-muted" aria-live="polite">
          {state === "connecting" ? "Connecting…" : "Connection lost: resuming your session…"}
        </p>
      )}
    </div>
  );
}
