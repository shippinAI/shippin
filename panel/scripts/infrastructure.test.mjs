import test from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import fs from "node:fs";
import yaml from "js-yaml";
import { loadNodes } from "../src/features/infrastructure/inventory.ts";

const node = { id: "11111111-1111-4111-8111-111111111111", name: "second-server", provider: "incus", status: "ready", schedulable: true, capabilities: ["containers"], readiness_reasons: [] };

test("inventory uses the canonical contract and handles actual HTTP success and failures", async (t) => {
  const spec = yaml.load(fs.readFileSync(new URL("../../api/openapi/openapi.yaml", import.meta.url), "utf8"));
  assert.equal(spec.paths["/operator/compute/nodes"].$ref, "paths/compute-nodes.yaml#/nodes");
  const paths = yaml.load(fs.readFileSync(new URL("../../api/openapi/paths/compute-nodes.yaml", import.meta.url), "utf8"));
  assert.equal(paths.nodes.get.operationId, "listComputeNodes");
  let mode = "success";
  const requests = [];
  const server = http.createServer((req, res) => {
    requests.push({ path: req.url, authorization: req.headers.authorization, cacheControl: req.headers["cache-control"] });
    res.setHeader("Content-Type", "application/json");
    if (mode === "denied") { res.writeHead(403); res.end(JSON.stringify({secret: "do not show"})); return; }
    if (mode === "malformed") { res.end(JSON.stringify({items: [{...node, schedulable: "yes"}]})); return; }
    res.end(JSON.stringify({items: mode === "empty" ? [] : [node]}));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  t.after(() => new Promise((resolve) => server.close(resolve)));
  const base = `http://127.0.0.1:${server.address().port}/v1`;
  assert.deepEqual(await loadNodes(base, " test-token "), [node]);
  assert.deepEqual(requests[0], { path: "/v1/operator/compute/nodes", authorization: "Bearer test-token", cacheControl: "no-cache" });
  mode = "empty"; assert.deepEqual(await loadNodes(base, "test-token"), []);
  mode = "denied"; await assert.rejects(loadNodes(base, "test-token"), (error) => /HTTP 403/.test(error.message) && !error.message.includes("do not show"));
  mode = "malformed"; await assert.rejects(loadNodes(base, "test-token"), /Invalid node inventory response/);
  const controller = new AbortController(); controller.abort();
  await assert.rejects(loadNodes(base, "test-token", controller.signal), { name: "AbortError" });
});
