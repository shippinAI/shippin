import "@xterm/xterm/css/xterm.css";
import { RotateCw } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { tokenValue } from "../../../../lib/design/token-value";
import type { TerminalBus } from "../../../../lib/terminal/terminal-bus";
import { useText } from "../../../../lib/ui-config/use-ui-config";
import type { TextKey } from "../../../../lib/ui-config/text-keys";
import { fitSize, maxAttempts, needsReopen, socketURL, TerminalConnection, type ConnState, type Mux, type SocketLike } from "../model";

type Props = { tenantId: string; workspaceId: string; bus: TerminalBus; mux?: Mux };
type Grid = { cols: number; rows: number; element?: HTMLElement };

/** What fits the pane: the cell xterm.js drew (its screen over its grid) into the pane's inner
 *  box, less the scrollbar. Null until xterm.js has drawn. */
function measure(el: HTMLElement, term: Grid) {
  const screen = term.element?.querySelector(".xterm-screen");
  const viewport = term.element?.querySelector<HTMLElement>(".xterm-viewport");
  if (!screen) return null;
  const box = screen.getBoundingClientRect();
  const st = getComputedStyle(el);
  const bar = viewport ? viewport.offsetWidth - viewport.clientWidth : 0;
  const w = el.clientWidth - parseFloat(st.paddingLeft) - parseFloat(st.paddingRight) - bar;
  const h = el.clientHeight - parseFloat(st.paddingTop) - parseFloat(st.paddingBottom);
  return fitSize(w, h, box.width / term.cols, box.height / term.rows);
}

/** xterm.js over the console's same-origin WebSocket (hostd PTY inside the workspace). It sizes
 *  itself to the pane and follows it, reconnects with backoff after a drop (back into the same
 *  Zellij session), and stops with the reason when the session was ended or refused. */
export function TerminalPane({ tenantId, workspaceId, bus, mux = "zellij" }: Props) {
  const text = useText();
  const host = useRef<HTMLDivElement>(null);
  const conn = useRef<TerminalConnection | null>(null);
  const [state, setState] = useState<ConnState>({ phase: "connecting" });
  const [fresh, setFresh] = useState(false); // show "Connected" briefly after (re)connecting
  useEffect(() => {
    const el = host.current;
    if (!el) return;
    let gone = false;
    let cleanup = () => {};
    void import("@xterm/xterm").then(({ Terminal }) => {
      if (gone) return;
      const term = new Terminal({
        cols: 80,
        rows: 24,
        cursorBlink: true,
        fontSize: 13,
        fontFamily: "var(--font-mono), monospace",
        theme: { background: tokenValue("--color-terminal") },
      });
      term.open(el);
      const first = measure(el, term);
      if (first) term.resize(first.cols, first.rows);
      let wasReconnecting = false,
        okTimer = 0,
        fitTimer = 0;
      const c = new TerminalConnection(
        {
          // WebSocket's handler types are wider than SocketLike's; the shapes match at runtime
          open: (rows, cols) => new WebSocket(socketURL(window.location, tenantId, workspaceId, rows, cols, mux)) as unknown as SocketLike,
          setTimer: (fn, ms) => window.setTimeout(fn, ms),
          clearTimer: (t) => window.clearTimeout(t as number),
        },
        {
          state: (s) => {
            setState(s);
            bus.attach(s.phase === "connected" ? (d) => c.send(d) : null);
            if (s.phase === "connected") {
              if (wasReconnecting && mux === "plain") term.write("\r\n\x1b[2m[reconnected: new shell]\x1b[0m\r\n");
              term.focus();
              setFresh(true);
              window.clearTimeout(okTimer);
              okTimer = window.setTimeout(() => setFresh(false), 2500);
            }
            if (s.phase === "closed") term.write("\r\n\x1b[2m[session closed]\x1b[0m\r\n");
            wasReconnecting = s.phase === "reconnecting" || (wasReconnecting && s.phase === "connecting");
          },
          output: (d) => term.write(d),
        },
      );
      conn.current = c;
      const sub = term.onData((d) => c.send(d));
      const ro = new ResizeObserver(() => {
        window.clearTimeout(fitTimer);
        fitTimer = window.setTimeout(() => {
          const next = measure(el, term);
          if (!next || (next.cols === term.cols && next.rows === term.rows)) return;
          term.resize(next.cols, next.rows);
          c.resize(next.rows, next.cols);
          if (needsReopen(c.state, mux, c.openedSize, next)) c.reconnect(); // the shell can't follow: reopen the session at this size
        }, 120);
      });
      ro.observe(el);
      c.start(term.rows, term.cols);
      cleanup = () => {
        ro.disconnect();
        window.clearTimeout(fitTimer);
        window.clearTimeout(okTimer);
        sub.dispose();
        bus.attach(null);
        c.dispose();
        conn.current = null;
        term.dispose();
      };
    });
    return () => {
      gone = true;
      cleanup();
    };
  }, [tenantId, workspaceId, bus, mux]);
  const closedText =
    state.phase === "closed" ? (state.kind === "refused" && state.message) || text(`terminal.closed.${state.kind}` as TextKey) : "";
  return (
    <div className="relative h-full min-h-0 overflow-hidden bg-terminal" data-state={state.phase}>
      <div ref={host} className="absolute inset-0 p-2" aria-label={text("terminal.region")} role="region" />
      {(state.phase !== "connected" || fresh) && (
        <div
          className="absolute right-3 top-2 flex max-w-md items-center gap-2 rounded-sm bg-raised px-2 py-0.5 text-xs text-fg-muted"
          aria-live="polite"
        >
          <span>
            {state.phase === "connecting" && text("terminal.connecting")}
            {state.phase === "connected" && text("terminal.connected")}
            {state.phase === "reconnecting" &&
              text("terminal.reconnecting", { seconds: Math.ceil(state.delayMs / 1000), attempt: state.attempt, max: maxAttempts })}
            {state.phase === "closed" && closedText}
          </span>
          {state.phase !== "connected" && state.phase !== "connecting" && (
            <Button size="sm" variant="secondary" tooltip={text("terminal.reconnectTooltip")} onPress={() => conn.current?.reconnect()}>
              <RotateCw className="size-4" /> {text("terminal.reconnect")}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
