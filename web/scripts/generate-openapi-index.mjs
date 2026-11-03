#!/usr/bin/env node
// Builds the API explorer's operation list from the published contract in api/openapi.
//   node scripts/generate-openapi-index.mjs          write it
//   node scripts/generate-openapi-index.mjs --check  fail if the committed file is out of date
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { load as parseYaml } from "js-yaml";

const here = path.dirname(fileURLToPath(import.meta.url));
const SOURCE = path.resolve(here, "../../api/openapi/openapi.yaml");
const OUTPUT = path.resolve(here, "../src/features/api-explorer/openapi-index.json");
const METHODS = new Set(["get", "post", "put", "patch", "delete", "head", "options"]);

const load = async (file) => parseYaml(await fs.readFile(file, "utf8"));

function pointerGet(doc, pointer) {
  let current = doc;
  for (const raw of pointer.slice(2).split("/")) {
    const token = raw.replace(/~1/g, "/").replace(/~0/g, "~");
    if (current == null || !(token in current)) throw new Error(`Pointer token not found: ${pointer}`);
    current = current[token];
  }
  return current;
}

async function resolveItem(item, dir) {
  if (item == null || typeof item !== "object" || !("$ref" in item)) return item;
  const at = item.$ref.indexOf("#");
  if (at === -1) throw new Error(`Unsupported $ref without pointer: ${item.$ref}`);
  return pointerGet(await load(path.resolve(dir, item.$ref.slice(0, at))), item.$ref.slice(at));
}

function operationsOf(pathKey, item) {
  if (item == null || typeof item !== "object") return [];
  return Object.entries(item)
    .filter(([method]) => METHODS.has(method))
    .map(([method, raw]) => {
      const op = raw != null && typeof raw === "object" ? raw : {};
      return {
        method: method.toUpperCase(),
        path: pathKey,
        operationId: op.operationId ?? `${method}:${pathKey}`,
        summary: op.summary ?? "",
        tags: Array.isArray(op.tags) ? op.tags : ["untagged"],
        implementationStatus: op["x-implementation-status"] ?? "unknown",
      };
    });
}

const doc = await load(SOURCE);
const items = await Promise.all(Object.entries(doc.paths ?? {}).map(async ([key, value]) => operationsOf(key, await resolveItem(value, path.dirname(SOURCE)))));
const operations = items.flat().sort((a, b) => a.tags[0].localeCompare(b.tags[0]) || a.path.localeCompare(b.path) || a.method.localeCompare(b.method));
// No timestamp: the same contract must produce the same bytes, so a build leaves the tree clean.
const text = `${JSON.stringify({
  basePath: doc.servers?.[0]?.url ?? "/v1",
  operationCount: operations.length,
  tags: [...new Set(operations.flatMap((o) => o.tags))].sort((a, b) => a.localeCompare(b)),
  operations,
}, null, 2)}\n`;

if (process.argv.includes("--check")) {
  const current = await fs.readFile(OUTPUT, "utf8").catch(() => "");
  if (current.replace(/\r\n/g, "\n") !== text) {
    console.error("openapi-index.json is out of date. Run: npm run generate:openapi");
    process.exit(1);
  }
  console.log(`openapi-index up to date (${operations.length} operations)`);
} else {
  await fs.writeFile(OUTPUT, text);
  console.log(`Generated ${operations.length} operations`);
}
