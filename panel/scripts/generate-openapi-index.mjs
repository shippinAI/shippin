import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import yaml from "js-yaml";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const OPENAPI_PATH = path.resolve(__dirname, "../../api/openapi/openapi.yaml");
const OUTPUT_PATH = path.resolve(
  __dirname,
  "../src/shared/generated/openapi-index.json"
);

const HTTP_METHODS = new Set([
  "get",
  "post",
  "put",
  "patch",
  "delete",
  "head",
  "options"
]);

async function loadYaml(filePath) {
  const raw = await fs.readFile(filePath, "utf8");
  return yaml.load(raw);
}

function pointerGet(documentObject, pointer) {
  if (!pointer.startsWith("#/")) {
    throw new Error(`Unsupported JSON pointer format: ${pointer}`);
  }
  const tokens = pointer
    .slice(2)
    .split("/")
    .map((token) => token.replace(/~1/g, "/").replace(/~0/g, "~"));
  let current = documentObject;
  for (const token of tokens) {
    if (current == null || !(token in current)) {
      throw new Error(`Pointer token not found: ${pointer}`);
    }
    current = current[token];
  }
  return current;
}

function parseRef(refValue) {
  const separatorIndex = refValue.indexOf("#");
  if (separatorIndex === -1) {
    throw new Error(`Unsupported $ref without pointer: ${refValue}`);
  }
  return {
    relativeFilePath: refValue.slice(0, separatorIndex),
    pointer: `#${refValue.slice(separatorIndex + 1)}`
  };
}

async function resolvePathItem(pathKey, pathValue, openapiDir) {
  if (pathValue != null && typeof pathValue === "object" && "$ref" in pathValue) {
    const { relativeFilePath, pointer } = parseRef(pathValue.$ref);
    const resolvedFile = path.resolve(openapiDir, relativeFilePath);
    const externalDoc = await loadYaml(resolvedFile);
    const dereferenced = pointerGet(externalDoc, pointer);
    return { pathKey, pathItem: dereferenced };
  }
  return { pathKey, pathItem: pathValue };
}

function extractOperations(pathKey, pathItem) {
  if (pathItem == null || typeof pathItem !== "object") {
    return [];
  }
  const operations = [];
  for (const [method, operation] of Object.entries(pathItem)) {
    if (!HTTP_METHODS.has(method)) {
      continue;
    }
    const normalizedOperation =
      operation != null && typeof operation === "object" ? operation : {};
    const tags = Array.isArray(normalizedOperation.tags)
      ? normalizedOperation.tags
      : ["untagged"];
    operations.push({
      method: method.toUpperCase(),
      path: pathKey,
      operationId: normalizedOperation.operationId ?? `${method}:${pathKey}`,
      summary: normalizedOperation.summary ?? "",
      tags,
      implementationStatus: normalizedOperation["x-implementation-status"] ?? "unknown"
    });
  }
  return operations;
}

async function main() {
  const openapi = await loadYaml(OPENAPI_PATH);
  const openapiDir = path.dirname(OPENAPI_PATH);
  const pathEntries = Object.entries(openapi.paths ?? {});
  const resolvedPathItems = await Promise.all(
    pathEntries.map(([pathKey, pathValue]) =>
      resolvePathItem(pathKey, pathValue, openapiDir)
    )
  );

  const operations = resolvedPathItems
    .flatMap(({ pathKey, pathItem }) => extractOperations(pathKey, pathItem))
    .sort((a, b) => {
      if (a.tags[0] !== b.tags[0]) {
        return a.tags[0].localeCompare(b.tags[0]);
      }
      if (a.path !== b.path) {
        return a.path.localeCompare(b.path);
      }
      return a.method.localeCompare(b.method);
    });

  const tagSet = new Set();
  for (const operation of operations) {
    for (const tag of operation.tags) {
      tagSet.add(tag);
    }
  }

  const output = {
    generatedAt: new Date().toISOString(),
    sourceFile: "../../api/openapi/openapi.yaml",
    operationCount: operations.length,
    tags: Array.from(tagSet).sort((a, b) => a.localeCompare(b)),
    operations
  };

  await fs.mkdir(path.dirname(OUTPUT_PATH), { recursive: true });
  await fs.writeFile(OUTPUT_PATH, `${JSON.stringify(output, null, 2)}\n`, "utf8");
  process.stdout.write(
    `Generated ${output.operationCount} operations into ${OUTPUT_PATH}\n`
  );
}

main().catch((error) => {
  process.stderr.write(`${error instanceof Error ? error.stack : String(error)}\n`);
  process.exitCode = 1;
});
