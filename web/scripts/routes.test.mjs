import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { generate } from "./routes.mjs";

// Builds a src/ folder from {"routes/a/page.tsx": "..."} and generates it.
function gen(files) {
  const src = mkdtempSync(join(tmpdir(), "routes-"));
  mkdirSync(join(src, "app"));
  for (const [p, text] of Object.entries(files)) {
    mkdirSync(join(src, p, ".."), { recursive: true });
    writeFileSync(join(src, p), text ?? "export default function P() { return null; }\n");
  }
  return generate(src);
}

const pathsOf = (text) => [...text.matchAll(/path: "([^"]+)"/g)].map((m) => m[1]);

test("folders are paths: parameters, the rest of the path and groups", () => {
  const { files, problems } = gen({
    "routes/web/layout.tsx": null,
    "routes/web/meta.ts": "export default { eager: true };\n",
    "routes/web/login/page.tsx": null,
    "routes/web/(app)/layout.tsx": null,
    "routes/web/(app)/page.tsx": null,
    "routes/web/(app)/t/[orgId]/items/page.tsx": null,
    "routes/web/(app)/t/[orgId]/items/new/page.tsx": null,
    "routes/web/(app)/t/[orgId]/items/[itemId]/page.tsx": null,
    "routes/web/(app)/docs/[...rest]/page.tsx": null,
  });
  assert.deepEqual(problems, []);
  const text = files.get("routes.web.gen.ts");
  // A fixed segment is tried before a parameter, and a page beside a group before the group.
  assert.deepEqual(pathsOf(text), ["/login", "/", "/docs/*", "/t/:orgId/items", "/t/:orgId/items/new", "/t/:orgId/items/:itemId"]);
  assert.match(text, /import P as m\d+|import m\d+ from "\.\.\/routes\/web\/layout";/);
  assert.match(text, /load: \(\) => import\("\.\.\/routes\/web\/\(app\)\/layout"\)/);
  assert.match(text, /\| "\/t\/:orgId\/items\/:itemId"/);
});

test("a page that only re-exports is loaded from its module", () => {
  const { files } = gen({
    "features/x/ui/x-page.tsx": "export function XPage() { return null; }\n",
    "routes/web/x/page.tsx": 'export { XPage as default } from "../../../features/x/ui/x-page";\n',
  });
  assert.match(files.get("routes.web.gen.ts"), /import\("\.\.\/features\/x\/ui\/x-page"\)\.then\(\(m\) => \(\{ default: m\.XPage \}\)\)/);
});

test("navigation entries are in their own file", () => {
  const { files } = gen({ "routes/web/x/page.tsx": null, "routes/web/x/meta.ts": "export default { nav: [] };\n" });
  assert.doesNotMatch(files.get("routes.web.gen.ts"), /from "[^"]*\/meta"/);
  assert.match(files.get("nav.web.gen.ts"), /\{ path: "\/x", page: true, meta: m0 \}/);
});

test("a module's pages join the panel's tree, and may not take its paths", () => {
  const ok = gen({ "routes/web/a/page.tsx": null, "modules/extra/routes/web/b/page.tsx": null });
  assert.deepEqual(ok.problems, []);
  assert.deepEqual(pathsOf(ok.files.get("routes.web.gen.ts")), ["/a", "/b"]);

  const clash = gen({ "routes/web/a/page.tsx": null, "modules/extra/routes/web/a/page.tsx": null });
  assert.equal(clash.problems.length, 1);
  assert.match(clash.problems[0], /module extra adds a page where the panel already has one/);
});

test("two folders serving one path are refused", () => {
  const { problems } = gen({ "routes/web/(a)/x/page.tsx": null, "routes/web/(b)/x/page.tsx": null });
  assert.equal(problems.length, 1);
  assert.match(problems[0], /web: \/x is served by both/);
});

test("a tree mounts another tree's routes where its folder is", () => {
  const { files, problems } = gen({
    "routes/panel/panel/layout.tsx": null,
    "routes/panel/panel/start/page.tsx": null,
    "routes/ops/(in)/layout.tsx": null,
    "routes/ops/(in)/[...rest]/page.tsx": null,
    "routes/ops/(in)/panel/mounts.ts": 'export default ["panel"];\n',
  });
  assert.deepEqual(problems, []);
  const ops = files.get("routes.ops.gen.ts");
  assert.match(ops, /import \{ routes as mount_panel \} from "\.\/routes\.panel\.gen";/);
  assert.ok(ops.indexOf("...mount_panel") < ops.indexOf('"/*"'), "the mounted tree is tried before the rest of the path");
  assert.match(gen({ "routes/a/x/mounts.ts": 'export default ["nope"];\n' }).problems[0], /mounts nope, which does not exist/);
});

test("route folder names are checked", () => {
  const { problems } = gen({ "routes/web/Bad_Name/page.tsx": null });
  assert.match(problems[0], /a route folder is lower-case words/);
});
