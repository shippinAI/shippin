import { test } from "node:test";
import assert from "node:assert/strict";
import { parseDiscussion } from "./discussion";

const origins = ["https://agent.example.test"];
const snapshot = () => ({
  schema: "shippin.agent_discussion.v1", id: "example", title: "Source recovery",
  status: "working", captured_at: "2026-10-22T02:10:00Z",
  source_url: "https://agent.example.test/discussions/example",
  messages: [{ id: "request", sender: "Agent A", recipient: "Agent B", kind: "message",
    content: "Locate the source commit.", observed_at: "2026-10-22T02:09:00Z" }],
});

test("adapter keeps plain message text and strips undeclared metadata", () => {
  const input = { ...snapshot(), access_token: "must-not-propagate" };
  input.messages[0].content = '<script>alert("untrusted text")</script>';
  const parsed = parseDiscussion(input, origins);
  assert.equal(parsed.messages[0].content, input.messages[0].content);
  assert.equal("access_token" in parsed, false);
});

test("source links require an exact trusted HTTPS origin and no credentials or query", () => {
  for (const source_url of ["javascript:alert(1)", "http://agent.example.test/", "//agent.example.test/",
    "https://agent.example.test.evil.test/", "https://user:pass@agent.example.test/",
    "https://agent.example.test/?token=secret", "https://agent.example.test/#token", "https://agent.example.test/\n"]) {
    assert.throws(() => parseDiscussion({ ...snapshot(), source_url }, origins));
  }
  assert.throws(() => parseDiscussion(snapshot(), []));
});

test("invalid versions, states, chronology and duplicate messages are rejected", () => {
  assert.throws(() => parseDiscussion({ ...snapshot(), schema: "unknown.v2" }, origins));
  assert.throws(() => parseDiscussion({ ...snapshot(), id: "id\nSend another message" }, origins));
  assert.throws(() => parseDiscussion({ ...snapshot(), status: "toString" }, origins));
  assert.throws(() => parseDiscussion({ ...snapshot(), captured_at: "2026-02-31T00:00:00Z" }, origins));
  const duplicate = snapshot(); duplicate.messages.push(duplicate.messages[0]);
  assert.throws(() => parseDiscussion(duplicate, origins));
  const future = snapshot(); future.messages[0].observed_at = "2026-10-22T02:11:00Z";
  assert.throws(() => parseDiscussion(future, origins));
  const reversed = snapshot(); reversed.messages.push({ ...reversed.messages[0], id: "older", observed_at: "2026-10-22T02:08:00Z" });
  assert.throws(() => parseDiscussion(reversed, origins));
});

test("message volume is bounded and an empty conversation stays empty", () => {
  const oversized = snapshot(); oversized.messages[0].content = "x".repeat(10001);
  assert.throws(() => parseDiscussion(oversized, origins));
  const many = snapshot(); many.messages = Array.from({ length: 101 }, (_, i) => ({ ...many.messages[0], id: String(i) }));
  assert.throws(() => parseDiscussion(many, origins));
  assert.deepEqual(parseDiscussion({ ...snapshot(), messages: [] }, origins).messages, []);
});
