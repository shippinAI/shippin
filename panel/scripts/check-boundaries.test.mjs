import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import ts from "typescript";
import { checkBoundaries } from "./check-boundaries.mjs";

function put(root, file, content) {
  const dest = path.join(root, file);
  fs.mkdirSync(path.dirname(dest), { recursive: true });
  fs.writeFileSync(dest, content);
}

test("allows composition and shared dependencies while rejecting reversed and cross-feature imports", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "shippin-boundaries-"));
  try {
    put(root, "shared/value.ts", "export const value = 1;");
    put(root, "features/a/a.ts", "import { value } from '../../shared/value'; export { value };");
    put(root, "features/b/b.ts", "export const b = 1;");
    put(root, "app/styles.css", "body { margin: 0; }");
    put(root, "app/app.ts", "import './styles.css'; import { value } from '../features/a/a'; export { value };");
    assert.deepEqual(checkBoundaries(root), []);
    put(root, "features/a/a.ts", "export { b } from '../b/b';");
    put(root, "shared/value.ts", "import '../app/app';");
    const errors = checkBoundaries(root).join("\n");
    assert.match(errors, /feature:a cannot import feature:b/);
    assert.match(errors, /shared cannot import app/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

test("checks aliases that resolve into the source tree", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "shippin-aliases-"));
  try {
    put(root, "features/a/a.ts", "import { b } from '@features/b/b'; export { b };");
    put(root, "features/b/b.ts", "export const b = 1;");
    const options = { module: ts.ModuleKind.ESNext, moduleResolution: ts.ModuleResolutionKind.Bundler,
      baseUrl: root, paths: { "@features/*": ["features/*"] } };
    assert.match(checkBoundaries(root, options).join("\n"), /feature:a cannot import feature:b/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

test("rejects nonliteral dynamic imports and unresolved relative imports", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "shippin-boundaries-"));
  try {
    put(root, "features/a/a.ts", "import './missing'; export const load = (x: string) => import(x);");
    const errors = checkBoundaries(root).join("\n");
    assert.match(errors, /unresolved import/);
    assert.match(errors, /dynamic module paths cannot be checked/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

test("rejects source names outside lowercase kebab-case", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "shippin-names-"));
  try {
    put(root, "features/MyFeature/FeatureView.tsx", "export const view = 1;");
    const errors = checkBoundaries(root).join("\n");
    assert.match(errors, /MyFeature.*lowercase kebab-case/);
    assert.match(errors, /FeatureView\.tsx.*lowercase kebab-case/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});
