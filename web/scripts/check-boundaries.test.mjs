import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const script = fileURLToPath(new URL("./check-boundaries.mjs", import.meta.url));

// Runs the check over a web/ folder built from {"src/…": "text"}.
function check(files) {
  const web = mkdtempSync(join(tmpdir(), "bounds-"));
  mkdirSync(join(web, "scripts"));
  for (const [p, text] of Object.entries(files)) {
    mkdirSync(join(web, p, ".."), { recursive: true });
    writeFileSync(join(web, p), text);
  }
  const r = spawnSync(process.execPath, [script, "--web", web], { encoding: "utf8" });
  return { ok: r.status === 0, out: r.stdout + r.stderr };
}

const base = {
  "src/features/a/index.ts": 'export { A } from "./ui/a";\n',
  "src/features/a/ui/a.tsx": "export const A = 1;\n",
  "src/lib/x.ts": "export const X = 1;\n",
};

test("a feature reaches another through its index.ts", () => {
  assert.ok(check({ ...base, "src/features/b/ui/b.tsx": 'import { A } from "../../a";\n' }).ok);
  const r = check({ ...base, "src/features/b/ui/b.tsx": 'import { A } from "../../a/ui/a";\n' });
  assert.ok(!r.ok);
  assert.match(r.out, /features\/b\/ui\/b\.tsx -> features\/a\/ui\/a\.tsx: reaches into another feature/);
});

test("the baseline lists older crossings and may only shrink", () => {
  const crossing = { ...base, "src/features/b/ui/b.tsx": 'import { A } from "../../a/ui/a";\n' };
  assert.ok(check({ ...crossing, "scripts/boundaries-baseline.txt": "features/b/ui/b.tsx -> features/a/ui/a.tsx\n" }).ok);
  const stale = check({ ...base, "scripts/boundaries-baseline.txt": "features/b/ui/b.tsx -> features/a/ui/a.tsx\n" });
  assert.ok(!stale.ok);
  assert.match(stale.out, /which is gone; remove the line/);
});

test("lib depends on no feature, and only generated files name a module", () => {
  const lib = check({ ...base, "src/lib/y.ts": 'import { A } from "../features/a";\n' });
  assert.match(lib.out, /src\/lib imports features\/a\/index\.ts/);
  assert.ok(check({ ...base, "src/lib/y.test.ts": 'import { A } from "../features/a";\n' }).ok, "a lib test may use a feature");
  const mod = check({
    ...base,
    "src/modules/m/x.ts": "export const M = 1;\n",
    "src/template/t.ts": 'import { M } from "../modules/m/x";\n',
  });
  assert.match(mod.out, /imports module code modules\/m\/x\.ts/);
  assert.ok(
    check({
      ...base,
      "src/modules/m/x.ts": "export const M = 1;\n",
      "src/app/r.gen.ts": 'export const r = () => import("../modules/m/x");\n',
    }).ok,
  );
});

test("a bucket folder is refused", () => {
  assert.match(check({ ...base, "src/lib/utils/u.ts": "export const U = 1;\n" }).out, /src\/lib\/utils: a bucket folder/);
});
