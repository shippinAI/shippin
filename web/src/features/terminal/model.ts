/** The workspace terminal's connection: a state machine over a WebSocket, kept free of the DOM
 *  and xterm.js so it can be tested. Protocol (internal/api/terminal_ws.go): binary frames are
 *  keystrokes and output; text frames are control, {"type":"resize"} from here and
 *  {"type":"ready","resize":bool} from the console; a close code says why a session ended. */

export type Mux = "zellij" | "plain";

/** Why a terminal is closed for good, until the person reconnects. */
export type CloseKind = "idle" | "maximum" | "revoked" | "stopped" | "ended" | "refused" | "gaveUp";

export type ConnState =
  | { phase: "connecting" }
  | { phase: "connected"; resize: boolean }
  | { phase: "reconnecting"; attempt: number; delayMs: number }
  | { phase: "closed"; kind: CloseKind; message: string };

export const maxAttempts = 8;
const baseDelayMs = 500;
const maxDelayMs = 8000;

/** Backoff before reconnect attempt n (1-based): 0.5 s, 1 s, 2 s, 4 s, then 8 s. */
export function retryDelay(attempt: number) {
  return Math.min(maxDelayMs, baseDelayMs * 2 ** (attempt - 1));
}

export type CloseVerdict = { retry: true } | { retry: false; kind: CloseKind; message: string };

/** What a close frame means. Session ends and refusals stop; drops and passing faults retry. */
export function classifyClose(code: number, reason: string): CloseVerdict {
  const fixed: Record<number, CloseKind> = { 4001: "idle", 4002: "maximum", 4003: "revoked", 4004: "stopped", 4005: "ended" };
  if (fixed[code]) return { retry: false, kind: fixed[code], message: "" };
  if (code === 4006) return { retry: true }; // the console could not observe the workspace just now
  if (code >= 4400 && code <= 4599) {
    // a refusal: 4000 + the HTTP status, the server's words as the reason
    if ([4502, 4503, 4504].includes(code)) return { retry: true };
    return { retry: false, kind: code === 4409 ? "stopped" : "refused", message: reason };
  }
  if (code >= 4000) return { retry: false, kind: "refused", message: reason };
  return { retry: true }; // 1006 (dropped, or refused before the handshake), 1001, 1011, 1012...
}

export function socketURL(loc: { protocol: string; host: string }, tenantId: string, workspaceId: string, rows: number, cols: number, mux: Mux) {
  const proto = loc.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${loc.host}/v1/tenants/${tenantId}/workspaces/${workspaceId}/terminal?rows=${rows}&cols=${cols}&mux=${mux}`;
}

/** Rows and columns that fit a pane, for a measured character cell; inside hostd's bounds. */
export function fitSize(width: number, height: number, cellW: number, cellH: number) {
  if (!(cellW > 0 && cellH > 0)) return null;
  return {
    cols: Math.min(1000, Math.max(20, Math.floor(width / cellW))),
    rows: Math.min(500, Math.max(5, Math.floor(height / cellH))),
  };
}

/** Without a real resize, reopen the Zellij session only when the size changed a lot. */
export function needsReopen(state: ConnState, mux: Mux, from: { rows: number; cols: number }, to: { rows: number; cols: number }) {
  if (state.phase !== "connected" || state.resize || mux !== "zellij") return false;
  return Math.abs(to.cols - from.cols) >= Math.max(8, from.cols * 0.1) || Math.abs(to.rows - from.rows) >= Math.max(4, from.rows * 0.1);
}

/** The slice of WebSocket this uses, so tests can drive it. */
export interface SocketLike {
  binaryType: string;
  readonly readyState: number;
  onmessage: ((e: { data: unknown }) => void) | null;
  onclose: ((e: { code: number; reason: string }) => void) | null;
  send(data: string | Uint8Array): void;
  close(): void;
}

export type ConnDeps = {
  open: (rows: number, cols: number) => SocketLike;
  setTimer: (fn: () => void, ms: number) => unknown;
  clearTimer: (t: unknown) => void;
};

export type ConnEvents = {
  state: (s: ConnState) => void;
  output: (data: Uint8Array) => void;
};

const OPEN = 1;

export class TerminalConnection {
  state: ConnState = { phase: "connecting" };
  private ws: SocketLike | null = null;
  private timer: unknown = null;
  private attempt = 0;
  private size = { rows: 24, cols: 80 };
  private opened = this.size;
  private gone = false;
  private enc = new TextEncoder();

  constructor(private deps: ConnDeps, private on: ConnEvents) {}

  start(rows: number, cols: number) {
    this.size = { rows, cols };
    this.connect();
  }

  /** Keystrokes; dropped while not connected (the shell is not there to take them). */
  send(data: string) {
    if (this.state.phase === "connected" && this.ws?.readyState === OPEN) this.ws.send(this.enc.encode(data));
  }

  /** The pane's size changed; sent now when connected, and used for the next open anyway. */
  resize(rows: number, cols: number) {
    this.size = { rows, cols };
    if (this.state.phase === "connected" && this.state.resize && this.ws?.readyState === OPEN) {
      this.ws.send(JSON.stringify({ type: "resize", rows, cols }));
    }
  }

  /** Connect again now: the person pressed Reconnect, or the size needs a fresh open. */
  reconnect() {
    if (this.gone) return;
    this.attempt = 0;
    this.drop();
    this.connect();
  }

  dispose() {
    this.gone = true;
    this.drop();
  }

  /** The size the current session was opened at (where a shell without resize is stuck). */
  get openedSize() {
    return this.opened;
  }

  private connect() {
    this.set({ phase: "connecting" });
    this.opened = this.size;
    const ws = this.deps.open(this.size.rows, this.size.cols);
    ws.binaryType = "arraybuffer";
    this.ws = ws;
    ws.onmessage = (e) => {
      if (this.ws !== ws) return;
      if (typeof e.data === "string") {
        if (parseReady(e.data) !== null) {
          this.attempt = 0;
          this.set({ phase: "connected", resize: parseReady(e.data) === true });
        }
        return;
      }
      this.on.output(new Uint8Array(e.data as ArrayBuffer));
    };
    ws.onclose = (e) => {
      if (this.ws !== ws || this.gone) return;
      this.ws = null;
      const v = classifyClose(e.code, e.reason);
      if (!v.retry) return this.set({ phase: "closed", kind: v.kind, message: v.message });
      if (++this.attempt > maxAttempts) return this.set({ phase: "closed", kind: "gaveUp", message: "" });
      const delayMs = retryDelay(this.attempt);
      this.set({ phase: "reconnecting", attempt: this.attempt, delayMs });
      this.timer = this.deps.setTimer(() => { this.timer = null; this.connect(); }, delayMs);
    };
  }

  private drop() {
    if (this.timer !== null) this.deps.clearTimer(this.timer);
    this.timer = null;
    const ws = this.ws;
    this.ws = null;
    ws?.close();
  }

  private set(s: ConnState) {
    this.state = s;
    this.on.state(s);
  }
}

/** The ready frame's resize flag, or null when the text frame is not a ready frame. */
function parseReady(text: string): boolean | null {
  try {
    const m = JSON.parse(text) as { type?: unknown; resize?: unknown };
    return m.type === "ready" ? m.resize === true : null;
  } catch {
    return null;
  }
}
