import assert from "node:assert/strict";
import {
  classifyClose,
  fitSize,
  maxAttempts,
  needsReopen,
  retryDelay,
  socketURL,
  TerminalConnection,
  type ConnState,
  type SocketLike,
} from "./model";

// Close codes: session ends and refusals stop with a reason; drops and passing faults retry.
assert.deepEqual(classifyClose(4001, "idle"), { retry: false, kind: "idle", message: "" });
assert.deepEqual(classifyClose(4002, "maximum"), { retry: false, kind: "maximum", message: "" });
assert.deepEqual(classifyClose(4003, "revoked"), { retry: false, kind: "revoked", message: "" });
assert.deepEqual(classifyClose(4004, "stopped"), { retry: false, kind: "stopped", message: "" });
assert.deepEqual(classifyClose(4005, "ended"), { retry: false, kind: "ended", message: "" });
assert.deepEqual(classifyClose(4006, "unverified"), { retry: true });
assert.deepEqual(classifyClose(4403, "Only the workspace's creator..."), {
  retry: false,
  kind: "refused",
  message: "Only the workspace's creator...",
});
assert.deepEqual(classifyClose(4404, "Workspace not found."), { retry: false, kind: "refused", message: "Workspace not found." });
assert.deepEqual(classifyClose(4409, "Start the workspace to open its terminal."), {
  retry: false,
  kind: "stopped",
  message: "Start the workspace to open its terminal.",
});
assert.deepEqual(classifyClose(4502, "The terminal didn't open."), { retry: true });
assert.deepEqual(classifyClose(4503, "audit"), { retry: true });
assert.deepEqual(classifyClose(4999, "new"), { retry: false, kind: "refused", message: "new" });
for (const code of [1000, 1001, 1006, 1011, 1012]) assert.deepEqual(classifyClose(code, ""), { retry: true });

assert.deepEqual([1, 2, 3, 4, 5, 6].map(retryDelay), [500, 1000, 2000, 4000, 8000, 8000]);

assert.equal(
  socketURL({ protocol: "https:", host: "c.example.test" }, "t1", "w1", 30, 100, "zellij"),
  "wss://c.example.test/v1/tenants/t1/workspaces/w1/terminal?rows=30&cols=100&mux=zellij",
);
assert.equal(
  socketURL({ protocol: "http:", host: "localhost:5173" }, "t", "w", 24, 80, "plain"),
  "ws://localhost:5173/v1/tenants/t/workspaces/w/terminal?rows=24&cols=80&mux=plain",
);

// Fit: whole cells only, inside hostd's bounds; nothing until a cell is measured.
assert.deepEqual(fitSize(800, 340, 8, 17), { cols: 100, rows: 20 });
assert.deepEqual(fitSize(10, 10, 8, 17), { cols: 20, rows: 5 });
assert.deepEqual(fitSize(1e6, 1e6, 1, 1), { cols: 1000, rows: 500 });
assert.equal(fitSize(800, 340, 0, 17), null);

const connected = (resize: boolean): ConnState => ({ phase: "connected", resize });
assert.equal(
  needsReopen(connected(true), "zellij", { rows: 24, cols: 80 }, { rows: 50, cols: 200 }),
  false,
  "a real resize needs no reopen",
);
assert.equal(
  needsReopen(connected(false), "zellij", { rows: 24, cols: 80 }, { rows: 25, cols: 83 }),
  false,
  "a small change keeps the session",
);
assert.equal(needsReopen(connected(false), "zellij", { rows: 24, cols: 80 }, { rows: 24, cols: 100 }), true);
assert.equal(needsReopen(connected(false), "zellij", { rows: 24, cols: 80 }, { rows: 40, cols: 80 }), true);
assert.equal(
  needsReopen(connected(false), "plain", { rows: 24, cols: 80 }, { rows: 40, cols: 120 }),
  false,
  "reopening a plain shell would lose it",
);

// The state machine, on fake sockets and timers.
class FakeSocket implements SocketLike {
  binaryType = "blob";
  readyState = 1;
  sent: (string | Uint8Array)[] = [];
  closed = false;
  onmessage: ((e: { data: unknown }) => void) | null = null;
  onclose: ((e: { code: number; reason: string }) => void) | null = null;
  constructor(
    public rows: number,
    public cols: number,
  ) {}
  send(d: string | Uint8Array) {
    this.sent.push(d);
  }
  close() {
    this.closed = true;
  }
  ready(resize: boolean) {
    this.onmessage?.({ data: JSON.stringify({ type: "ready", resize }) });
  }
  drop(code = 1006, reason = "") {
    this.readyState = 3;
    this.onclose?.({ code, reason });
  }
}

function rig() {
  const sockets: FakeSocket[] = [];
  const timers: { fn: () => void; ms: number; cleared: boolean }[] = [];
  const states: ConnState[] = [];
  const output: string[] = [];
  const c = new TerminalConnection(
    {
      open: (rows, cols) => {
        const s = new FakeSocket(rows, cols);
        sockets.push(s);
        return s;
      },
      setTimer: (fn, ms) => {
        const t = { fn, ms, cleared: false };
        timers.push(t);
        return t;
      },
      clearTimer: (t) => {
        (t as { cleared: boolean }).cleared = true;
      },
    },
    { state: (s) => states.push(s), output: (d) => output.push(new TextDecoder().decode(d)) },
  );
  const fire = () => {
    const t = timers.at(-1)!;
    assert.equal(t.cleared, false);
    t.fn();
  };
  return { c, sockets, timers, states, output, fire, last: () => sockets.at(-1)! };
}

{
  // connect, type, resize, output; then a drop reconnects with backoff at the latest size
  const r = rig();
  r.c.start(30, 100);
  assert.deepEqual(r.c.state, { phase: "connecting" });
  assert.equal(r.last().binaryType, "arraybuffer");
  r.c.send("too early"); // nothing is sent before the console says ready
  assert.equal(r.last().sent.length, 0);
  r.last().ready(true);
  assert.deepEqual(r.c.state, { phase: "connected", resize: true });
  r.c.send("ls\r");
  r.c.resize(40, 120);
  assert.equal(new TextDecoder().decode(r.last().sent[0] as Uint8Array), "ls\r");
  assert.equal(r.last().sent[1], JSON.stringify({ type: "resize", rows: 40, cols: 120 }));
  r.last().onmessage?.({ data: new TextEncoder().encode("hello").buffer });
  assert.deepEqual(r.output, ["hello"]);
  r.last().drop();
  assert.deepEqual(r.c.state, { phase: "reconnecting", attempt: 1, delayMs: 500 });
  r.fire();
  assert.equal(r.sockets.length, 2);
  assert.deepEqual([r.last().rows, r.last().cols], [40, 120], "the new session opens at the latest size");
  r.last().drop();
  assert.deepEqual(r.c.state, { phase: "reconnecting", attempt: 2, delayMs: 1000 });
  r.fire();
  r.last().ready(true); // a successful reconnect resets the backoff
  r.last().drop();
  assert.deepEqual(r.c.state, { phase: "reconnecting", attempt: 1, delayMs: 500 });
}

{
  // a refusal (accepted, then closed with 4403) stops at once and keeps the server's words
  const r = rig();
  r.c.start(24, 80);
  r.last().drop(4403, "Only the workspace's creator or a tenant owner or admin can open its terminal.");
  assert.deepEqual(r.c.state, {
    phase: "closed",
    kind: "refused",
    message: "Only the workspace's creator or a tenant owner or admin can open its terminal.",
  });
  assert.equal(r.timers.length, 0, "no retry after a refusal");
}

{
  // session ends stop too: idle, the time limit, revoked access
  for (const [code, kind] of [
    [4001, "idle"],
    [4002, "maximum"],
    [4003, "revoked"],
    [4005, "ended"],
  ] as const) {
    const r = rig();
    r.c.start(24, 80);
    r.last().ready(true);
    r.last().drop(code, kind);
    assert.deepEqual(r.c.state, { phase: "closed", kind, message: "" });
    assert.equal(r.timers.length, 0);
  }
}

{
  // a console that cannot observe the workspace (4006) or hostd that did not open (4502) retries
  const r = rig();
  r.c.start(24, 80);
  r.last().drop(4502, "The terminal didn't open.");
  assert.equal(r.c.state.phase, "reconnecting");
  r.fire();
  r.last().drop(4006, "unverified");
  assert.equal(r.c.state.phase, "reconnecting");
}

{
  // it gives up after maxAttempts drops in a row
  const r = rig();
  r.c.start(24, 80);
  for (let i = 1; i <= maxAttempts; i++) {
    r.last().drop();
    assert.equal(r.c.state.phase, "reconnecting");
    r.fire();
  }
  r.last().drop();
  assert.deepEqual(r.c.state, { phase: "closed", kind: "gaveUp", message: "" });
  // Reconnect starts over
  r.c.reconnect();
  assert.equal(r.c.state.phase, "connecting");
  r.last().drop();
  assert.deepEqual(r.c.state, { phase: "reconnecting", attempt: 1, delayMs: 500 });
}

{
  // without a real resize the size is remembered, not sent; reconnect closes the old socket quietly
  const r = rig();
  r.c.start(24, 80);
  r.last().ready(false);
  r.c.resize(40, 120);
  assert.equal(r.last().sent.length, 0);
  const old = r.last();
  r.c.reconnect();
  assert.equal(old.closed, true);
  old.drop(1000); // its close event must not schedule a retry
  assert.equal(r.timers.length, 0);
  assert.deepEqual([r.last().rows, r.last().cols], [40, 120]);
  assert.deepEqual(r.c.openedSize, { rows: 40, cols: 120 });
}

{
  // dispose closes the socket and cancels a pending retry; nothing happens after it
  const r = rig();
  r.c.start(24, 80);
  r.last().drop();
  const pending = r.timers.at(-1)!;
  r.c.dispose();
  assert.equal(pending.cleared, true);
  const n = r.states.length;
  r.c.reconnect();
  assert.equal(r.states.length, n);
}

console.log("terminal model ok");
