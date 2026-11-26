import assert from "node:assert/strict";
import { QueryClient } from "@tanstack/react-query";
import { guestInventoryQuery } from "./use-hosts";

const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } });
const originalFetch = globalThis.fetch;
let items = [{ id: "existing-vm", name: "Existing VM", kind: "vm", status: "running", node: "node-a" }];
let failed = false;
const requests: string[] = [];
globalThis.fetch = async (input) => {
  requests.push(String(input));
  return new Response(JSON.stringify(failed ? { message: "Provider unavailable" } : { items }), {
    status: failed ? 502 : 200,
    headers: { "Content-Type": "application/json" },
  });
};
try {
  const a = guestInventoryQuery("/v1/infra/servers/host-a");
  const b = guestInventoryQuery("/v1/infra/servers/host-b");
  const first = await qc.fetchQuery(a);
  assert.equal(first.items[0]?.id, "existing-vm", "pre-existing guests do not need a local create/import record");
  items = [...items, { id: "external-vm", name: "Created elsewhere", kind: "vm", status: "stopped", node: "node-a" }];
  assert.equal((await qc.fetchQuery(a)).items.length, 2, "a later provider addition appears on refresh");
  items = items.filter((g) => g.id === "external-vm");
  assert.deepEqual(
    (await qc.fetchQuery(a)).items.map((g) => g.id),
    ["external-vm"],
    "provider removal disappears on refresh",
  );
  failed = true;
  await assert.rejects(qc.fetchQuery(a), /Provider unavailable/);
  const stale = qc.getQueryState(a.queryKey);
  assert.equal(stale?.status, "error", "a refresh failure remains an error even with cached rows");
  assert.deepEqual(qc.getQueryData(a.queryKey), { items }, "last-known rows can be labelled, not replaced with fake empty success");
  assert.equal(qc.getQueryData(b.queryKey), undefined, "another host never inherits this host's inventory");
  failed = false;
  items = [];
  assert.deepEqual(await qc.fetchQuery(a), { items: [] }, "recovery can legitimately report no guests");
  assert.equal(qc.getQueryState(a.queryKey)?.status, "success");
  assert.ok(requests.every((p) => p === "/v1/operator/infrastructure/servers/host-a/guests"));
  assert.equal(guestInventoryQuery("").enabled, false, "no host means no discovery request");
} finally {
  globalThis.fetch = originalFetch;
  qc.clear();
}
console.log("dynamic guest inventory ok");
