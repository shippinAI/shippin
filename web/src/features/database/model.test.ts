import { test } from "node:test";
import assert from "node:assert/strict";
import { hintForSQL, isUserTable, rowCountLabel, userTables, containsFilter } from "./model";

test("vaultix and platform tables are not the empty-state database", () => {
  const tables = [
    { schema: "public", name: "secrets_refs", platform: true },
    { schema: "public", name: "todos", platform: false },
    { schema: "vx_sid", name: "items", platform: false },
    { schema: "vaultix", name: "secrets", platform: false },
  ];
  assert.deepEqual(userTables(tables).map((t) => t.name), ["todos"]);
  assert.equal(isUserTable({ schema: "vx_sid", name: "items", platform: false }), false);
});

test("row counts stay plain", () => {
  assert.equal(rowCountLabel({ row_count: 0, estimate: 10 }), "0 rows");
  assert.equal(rowCountLabel({ row_count: 1, estimate: 0 }), "1 row");
  assert.equal(rowCountLabel({ row_count: null, estimate: 0 }), "empty");
});

test("sql hints name the effect", () => {
  assert.match(hintForSQL(""), /SELECT/);
  assert.match(hintForSQL("select * from todos"), /reads data/);
  assert.match(hintForSQL("DELETE FROM todos"), /confirm/);
  assert.match(hintForSQL("  update todos set done = true"), /WHERE/);
});

test("contains filter does not treat % as a wildcard typed by accident", () => {
  assert.deepEqual(containsFilter("100%"), { op: "ilike", value: "%100\\%%" });
});
