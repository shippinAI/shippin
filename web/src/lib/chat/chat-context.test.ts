// One ChatSurface, more UI by route (jiti runs the TS).
import { test } from "node:test";
import assert from "node:assert/strict";
import { chatContextFor } from "./chat-context";

const W = "/t/t1/workspaces/w1";

test("the /chat page shows the session rail and pops into the overlay", () => {
  const c = chatContextFor("/chat/abc", "page");
  assert.equal(c.kind, "chat");
  assert.equal(c.sessions, true);
  assert.equal(c.swap, "pop");
});

test("the overlay has no rail, expands to /chat, and adds route context", () => {
  const t = chatContextFor(`${W}/terminal`, "overlay");
  assert.deepEqual([t.kind, t.sessions, t.swap, t.tenantId, t.workspaceId], ["terminal", false, "expand", "t1", "w1"]);
  assert.ok(t.prompts.some((p) => p.includes("Zellij")));
  assert.equal(chatContextFor(`${W}/connect`, "overlay").kind, "connect");
  assert.equal(chatContextFor(`${W}/publish`, "overlay").kind, "publish");
  assert.equal(chatContextFor(W, "overlay").kind, "workspace");
  assert.equal(chatContextFor("/t/t1/projects", "overlay").kind, "general");
});
