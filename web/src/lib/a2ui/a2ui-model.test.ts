// A2UI v0.9 surface state machine (jiti runs the TS).
import { test } from "node:test";
import assert from "node:assert/strict";
import { applyAll, getPath, parseJSONL, resolve, setPath } from "./a2ui-model";
import { SHIPPIN_CATALOG, type A2UIMessage } from "./a2ui-types";

const basic = "https://a2ui.org/specification/v0_9/catalogs/basic/catalog.json";
const spec = parseJSONL(`{"version":"v0.9","createSurface":{"surfaceId":"main","catalogId":"${basic}"}}
{"version":"v0.9","updateComponents":{"surfaceId":"main","components":[{"id":"root","component":"Column","children":["t"]},{"id":"t","component":"Text","text":{"path":"/user/name"}}]}}
not json
{"version":"v0.9","updateDataModel":{"surfaceId":"main","path":"/user","value":{"name":"Alice"}}}`);

test("parses the spec's JSONL example and skips bad lines", () => {
  assert.equal(spec.length, 3);
  const s = applyAll(spec).main;
  assert.equal(s.components.root.component, "Column");
  assert.equal(resolve((s.components.t as { text: { path: string } }).text, s.data), "Alice");
});

test("unknown catalogs, unknown components and wrong versions are dropped", () => {
  const ms = [
    { version: "v0.9", createSurface: { surfaceId: "x", catalogId: "https://evil.example/catalog" } },
    { version: "v0.9", createSurface: { surfaceId: "y", catalogId: SHIPPIN_CATALOG } },
    { version: "v0.9", updateComponents: { surfaceId: "y", components: [{ id: "root", component: "Script", src: "x" }, { id: "ok", component: "Divider" }] } },
    { version: "v0.8", deleteSurface: { surfaceId: "y" } },
  ] as unknown as A2UIMessage[];
  const s = applyAll(ms);
  assert.equal(s.x, undefined);
  assert.deepEqual(Object.keys(s.y.components), ["ok"]);
});

test("createSurface twice keeps the first; deleteSurface removes it; updates to missing surfaces are ignored", () => {
  const c: A2UIMessage = { version: "v0.9", createSurface: { surfaceId: "a", catalogId: SHIPPIN_CATALOG } };
  const u: A2UIMessage = { version: "v0.9", updateDataModel: { surfaceId: "a", value: { n: 1 } } };
  assert.deepEqual(applyAll([c, u, c]).a.data, { n: 1 });
  assert.equal(applyAll([c, { version: "v0.9", deleteSurface: { surfaceId: "a" } }]).a, undefined);
  assert.deepEqual(applyAll([u]), {});
});

test("JSON pointers: absolute, relative scope, escapes, upsert and remove", () => {
  const d = { list: [{ name: "a" }, { name: "b" }], "x/y": 1 };
  assert.equal(getPath(d, "name", "/list/1"), "b");
  assert.equal(getPath(d, "/x~1y"), 1);
  assert.deepEqual(setPath(d, "/list/0/name", "z"), { list: [{ name: "z" }, { name: "b" }], "x/y": 1 });
  assert.deepEqual(setPath({ a: 1, b: 2 }, "/a", undefined), { b: 2 });
  assert.deepEqual(setPath({}, "/new/deep", true), { new: { deep: true } });
  assert.equal(d.list[0].name, "a"); // immutable
});
