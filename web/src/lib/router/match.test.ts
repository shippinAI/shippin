// npm run test:router (jiti runs the TS): route matching parity
// with the react-router behaviour the console relied on.
import { test } from "node:test";
import assert from "node:assert/strict";
import { matchRoutes, type RouteObject } from "./match";

const routes: RouteObject[] = [
  { path: "/login" },
  {
    children: [
      { path: "/", index: true },
      { path: "/admin", children: [{ index: true }, { path: "customers" }, { path: "customers/:id" }] },
      { path: "/t/:tenantId", children: [{ index: true }, { path: "workspaces/:wsId" }, { path: "files/*" }] },
    ],
  },
  { path: "*" },
];
const ids = (p: string) => matchRoutes(routes, p)?.map((m) => m.route.path ?? (m.route.index ? "#" : "()"));
const params = (p: string) => Object.assign({}, ...(matchRoutes(routes, p) ?? []).map((m) => m.params));

test("absolute, index and nested routes", () => {
  assert.deepEqual(ids("/login"), ["/login"]);
  assert.deepEqual(ids("/admin"), ["()", "/admin", "#"]);
  assert.deepEqual(ids("/admin/customers"), ["()", "/admin", "customers"]);
  assert.deepEqual(params("/admin/customers/c%201"), { id: "c 1" });
});
test("params and splats", () => {
  assert.deepEqual(params("/t/acme/workspaces/w1"), { tenantId: "acme", wsId: "w1" });
  assert.deepEqual(params("/t/acme/files/a/b"), { tenantId: "acme", "*": "a/b" });
});
test("unmatched falls through to the catch-all", () => {
  assert.deepEqual(ids("/t/acme/nope"), ["*"]);
  assert.deepEqual(ids("/admin/customers/x/y"), ["*"]);
  assert.equal(matchRoutes([{ path: "/a" }], "/b"), null);
});
