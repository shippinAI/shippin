import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import yaml from "js-yaml";

const repository = new URL("../../", import.meta.url);
const apiRoot = new URL("../../api/openapi/", import.meta.url);
const specUrl = new URL("openapi.yaml", apiRoot);

function yamlFiles(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const target = path.join(directory, entry.name);
    return entry.isDirectory() ? yamlFiles(target) : /\.ya?ml$/i.test(entry.name) ? [target] : [];
  });
}

function pointerValue(root, pointer) {
  return pointer.split("/").slice(1).reduce((value, token) => {
    const key = token.replace(/~1/g, "/").replace(/~0/g, "~");
    return value?.[key];
  }, root);
}

function visit(value, visitObject) {
  if (!value || typeof value !== "object") return;
  visitObject(value);
  for (const child of Object.values(value)) visit(child, visitObject);
}

test("OpenAPI uses the configured Nomen identity contract consistently", () => {
  const files = yamlFiles(fileURLToPath(apiRoot));
  const spec = yaml.load(fs.readFileSync(specUrl, "utf8"));
  const schemes = spec.components.securitySchemes;

  assert.equal(spec.info.title, "Shippin.app Platform API");
  assert.ok(schemes.nomen, "Nomen OIDC security scheme is missing");
  assert.ok(!schemes.zitadel, "legacy Zitadel security scheme remains published");

  for (const file of files) {
    const source = fs.readFileSync(file, "utf8");
    assert.doesNotMatch(source, /zitadel/i, `${path.relative(fileURLToPath(repository), file)} has stale identity naming`);
    const document = yaml.load(source);
    visit(document, (object) => {
      for (const security of object.security ?? []) {
        for (const name of Object.keys(security)) assert.ok(schemes[name], `unknown security scheme ${name} in ${file}`);
      }
      if (typeof object.$ref !== "string") return;
      const [fileRef, pointer = ""] = object.$ref.split("#");
      if (!fileRef) return;
      const targetPath = path.resolve(path.dirname(file), fileRef);
      assert.ok(fs.existsSync(targetPath), `missing $ref target ${object.$ref} in ${file}`);
      if (!pointer) return;
      const targetText = fs.readFileSync(targetPath, "utf8");
      const target = /\.json$/i.test(targetPath) ? JSON.parse(targetText) : yaml.load(targetText);
      assert.notEqual(pointerValue(target, pointer), undefined, `missing $ref pointer ${object.$ref} in ${file}`);
    });
  }
});
