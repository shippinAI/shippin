import "@xterm/xterm/css/xterm.css";
import { useEffect, useRef, useState } from "react";
import type { TerminalBus } from "../../../lib/terminal/terminal-bus";
import { tokenValue } from "../../../lib/design/token-value";

type Mux = "zellij" | "plain";
type Props = { tenantId: string; workspaceId: string; bus: TerminalBus; mux?: Mux };

/** Font metrics used to size rows/cols from the pane before the socket opens. */
const cell = { w: 7.8, h: 17 };

export function terminalSocketURL(loc: Location, tenantId: string, workspaceId: string, rows: number, cols: number, mux: Mux = "zellij") {
  const proto = loc.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${loc.host}/v1/tenants/${tenantId}/workspaces/${workspaceId}/terminal?rows=${rows}&cols=${cols}&mux=${mux}`;
}

/** xterm.js over the console's same-origin WebSocket (hostd PTY inside the workspace). */
export function TerminalPane({ tenantId, workspaceId, bus, mux = "zellij" }: Props) {
  const host = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<"connecting" | "open" | "closed">("connecting");
  useEffect(() => {
    const el = host.current;
    if (!el) return;
    let stop = () => {};
    let gone = false;
    void import("@xterm/xterm").then(({ Terminal }) => {
      if (gone) return;
    const cols = Math.max(40, Math.floor(el.clientWidth / cell.w));
    const rows = Math.max(10, Math.floor(el.clientHeight / cell.h));
    const term = new Terminal({ cols, rows, cursorBlink: true, fontSize: 13, fontFamily: "var(--font-mono), monospace",
      theme: { background: tokenValue("--color-terminal") } });
    term.open(el);
    const ws = new WebSocket(terminalSocketURL(window.location, tenantId, workspaceId, rows, cols, mux));
    ws.binaryType = "arraybuffer";
    const enc = new TextEncoder();
    const send = (d: string) => ws.readyState === WebSocket.OPEN && ws.send(enc.encode(d));
    ws.onopen = () => { setState("open"); bus.attach(send); term.focus(); };
    ws.onmessage = (e) => term.write(typeof e.data === "string" ? e.data : new Uint8Array(e.data as ArrayBuffer));
    ws.onclose = () => { setState("closed"); bus.attach(null); term.write("\r\n\x1b[2m[session closed]\x1b[0m\r\n"); };
    const sub = term.onData(send);
    stop = () => { sub.dispose(); bus.attach(null); ws.close(); term.dispose(); };
    });
    return () => { gone = true; stop(); };
  }, [tenantId, workspaceId, bus, mux]);
  return (
    <div className="relative h-full min-h-0 overflow-hidden bg-terminal">
      <div ref={host} className="absolute inset-0 p-2" aria-label="Workspace terminal" role="region" />
      {state !== "open" && (
        <p className="absolute right-3 top-2 rounded-sm bg-raised px-2 py-0.5 text-xs text-fg-muted" aria-live="polite">
          {state === "connecting" ? "Connecting…" : "Disconnected"}
        </p>
      )}
    </div>
  );
}
