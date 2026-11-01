import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, readFileSync, realpathSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname, basename } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";

test("inline export preserves literal messages without allowing script termination", () => {
  const folder = mkdtempSync(join(tmpdir(), "shippin-discussion-"));
  try {
    const content = '</script><script>alert("untrusted")</script> & $&';
    const snapshot = { schema: "shippin.agent_discussion.v1", id: "test", title: "Test",
      status: "complete", captured_at: "2026-10-22T02:00:00Z", source_url: "https://agent.example.test/chat",
      messages: [{ id: "one", sender: "A", recipient: "B", kind: "message", content, observed_at: "2026-10-22T02:00:00Z" }] };
    const input = join(folder, "snapshot.json"), output = join(folder, "discussion.html");
    writeFileSync(input, JSON.stringify(snapshot));
    execFileSync(process.execPath, [fileURLToPath(new URL("./render-inline-discussion.mjs", import.meta.url)), input, output, "https://agent.example.test"]);
    const fragment = readFileSync(output, "utf8");
    const data = fragment.match(/<script type="application\/json" data-discussion>([\s\S]*?)<\/script>/)?.[1];
    assert.ok(data);
    assert.equal(JSON.parse(data).messages[0].content, content);
    assert.doesNotMatch(fragment, /<script>alert/);
    assert.doesNotMatch(fragment, /__DISCUSSION_DATA__/);
  } finally {
    const target = realpathSync(folder), parent = realpathSync(tmpdir());
    if (dirname(target) !== parent || !basename(target).startsWith("shippin-discussion-")) throw new Error("Unsafe test cleanup target");
    rmSync(target, { recursive: true, force: true });
  }
});
