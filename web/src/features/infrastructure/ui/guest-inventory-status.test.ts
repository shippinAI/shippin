import assert from "node:assert/strict";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createJiti } from "jiti";

const tsx = createJiti(import.meta.url, { jsx: { runtime: "automatic" } });
const { GuestInventoryStatus } = await tsx.import<typeof import("./guest-inventory-status")>("./guest-inventory-status.tsx");

const qc = new QueryClient({ defaultOptions: { queries: { gcTime: Infinity } } });
qc.setQueryData(["ui-config"], {});
const render = (updatedAt: number, error: Error | null, refreshing = false) =>
  renderToStaticMarkup(
    createElement(
      QueryClientProvider,
      { client: qc },
      createElement(GuestInventoryStatus, { updatedAt, error, refreshing, onRefresh() {} }),
    ),
  );
const stale = render(1_700_000_000_000, new Error("Host is offline"));
assert.match(stale, /role="alert"/);
assert.match(stale, /last known machines/);
assert.match(stale, /Last successful refresh/);
assert.match(stale, /Host is offline/);
assert.match(stale, /Refresh inventory/);
const missing = render(0, new Error("Access denied"));
assert.match(missing, /Inventory could not be loaded/);
assert.doesNotMatch(missing, /Last successful refresh/);
const refreshing = render(0, null, true);
assert.match(refreshing, /disabled/);
assert.match(refreshing, /Refreshing inventory/);
assert.doesNotMatch(render(1_700_000_000_000, null), /role="alert"/);
qc.clear();
console.log("guest inventory status ok");
