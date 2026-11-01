// WezTerm config download (jiti runs the TS).
import { test } from "node:test";
import assert from "node:assert/strict";
import { domainName, weztermConfig } from "./wezterm-config";

test("ssh mux + plain ssh domains to the workspace, both starting Zellij", () => {
  const c = weztermConfig({ workspace: "api", host: "203.0.113.7", port: 2201, user: "dev" });
  assert.match(c, /name = 'shippin-api',\n\s+remote_address = '203\.0\.113\.7:2201',\n\s+username = 'dev',\n\s+multiplexing = 'WezTerm'/);
  assert.match(c, /name = 'shippin-api-ssh'[\s\S]*multiplexing = 'None'/);
  assert.equal(c.match(/shippin-term/g)?.length, 2);
  assert.match(c, /wezterm connect shippin-api/);
});

test("names and values cannot break out of Lua strings or comments", () => {
  assert.equal(domainName("My WS!"), "shippin-my-ws");
  const c = weztermConfig({ workspace: "x\nos.execute('rm')", host: "h'); os.exit()--", port: 22, user: "u" });
  assert.ok(!c.includes("\nos.execute"));
  assert.match(c, /remote_address = 'h\\'\); os\.exit\(\)--:22'/);
});
