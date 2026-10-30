import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const scriptPath = fileURLToPath(import.meta.url);
const defaultSource = path.resolve(path.dirname(scriptPath), "../src");

function sourceFiles(root) {
  return fs.readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    const name = path.join(root, entry.name);
    return entry.isDirectory() ? sourceFiles(name) : /\.tsx?$/.test(entry.name) ? [name] : [];
  });
}

function checkNames(root, current = root) {
  const errors = [];
  for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
    const filename = path.join(current, entry.name);
    const valid = entry.isDirectory()
      ? /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(entry.name)
      : /^[a-z0-9]+(?:-[a-z0-9]+)*(?:\.d)?\.(?:ts|tsx|css|json)$/.test(entry.name);
    if (!valid) errors.push(`${path.relative(root, filename)}: source names must be lowercase kebab-case`);
    if (entry.isDirectory()) errors.push(...checkNames(root, filename));
  }
  return errors;
}

function category(root, filename) {
  const parts = path.relative(root, filename).split(path.sep);
  if (parts[0] === "shared") return "shared";
  if (parts[0] === "app") return "app";
  if (parts[0] === "features" && parts.length >= 3) return `feature:${parts[1]}`;
  return "entry";
}

function imports(sourceFile) {
  const found = [];
  function visit(node) {
    if ((ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) && node.moduleSpecifier) {
      found.push(node.moduleSpecifier);
    } else if (ts.isImportEqualsDeclaration(node) && ts.isExternalModuleReference(node.moduleReference)) {
      found.push(node.moduleReference.expression);
    } else if (ts.isCallExpression(node) &&
      (node.expression.kind === ts.SyntaxKind.ImportKeyword ||
        (ts.isIdentifier(node.expression) && node.expression.text === "require"))) {
      found.push(node.arguments[0]);
    }
    ts.forEachChild(node, visit);
  }
  visit(sourceFile);
  return found;
}

export function checkBoundaries(root, options = { moduleResolution: ts.ModuleResolutionKind.Bundler, module: ts.ModuleKind.ESNext, resolveJsonModule: true }) {
  const errors = checkNames(root);
  for (const file of sourceFiles(root)) {
    const source = ts.createSourceFile(file, fs.readFileSync(file, "utf8"), ts.ScriptTarget.Latest, true);
    for (const specifier of imports(source)) {
      if (!specifier || !ts.isStringLiteralLike(specifier)) {
        errors.push(`${path.relative(root, file)}: dynamic module paths cannot be checked`);
        continue;
      }
      const isRelative = specifier.text.startsWith(".");
      let target = ts.resolveModuleName(specifier.text, file, options, ts.sys).resolvedModule?.resolvedFileName;
      if (!target && isRelative && specifier.text.endsWith(".css")) {
        const asset = path.resolve(path.dirname(file), specifier.text);
        if (fs.existsSync(asset)) target = asset;
      }
      if (!target) {
        if (isRelative) errors.push(`${path.relative(root, file)}: unresolved import ${specifier.text}`);
        continue;
      }
      const relativeTarget = path.relative(root, target);
      if (relativeTarget === ".." || relativeTarget.startsWith(`..${path.sep}`) || path.isAbsolute(relativeTarget)) {
        if (isRelative) errors.push(`${path.relative(root, file)}: import escapes src: ${specifier.text}`);
        continue;
      }
      const from = category(root, file), to = category(root, target);
      if ((from === "shared" && to !== "shared") ||
        (from.startsWith("feature:") && (to === "app" || to === "entry" || (to.startsWith("feature:") && from !== to)))) {
        errors.push(`${path.relative(root, file)}: ${from} cannot import ${to} (${specifier.text})`);
      }
    }
  }
  return errors;
}

if (process.argv[1] && path.resolve(process.argv[1]) === scriptPath) {
  const configPath = path.resolve(path.dirname(scriptPath), "../tsconfig.json");
  const config = ts.readConfigFile(configPath, ts.sys.readFile);
  if (config.error) {
    console.error(ts.flattenDiagnosticMessageText(config.error.messageText, "\n"));
    process.exitCode = 1;
  } else {
    const options = ts.parseJsonConfigFileContent(config.config, ts.sys, path.dirname(configPath)).options;
    const errors = checkBoundaries(defaultSource, options);
    if (errors.length) { console.error(errors.join("\n")); process.exitCode = 1; }
    else console.log("Panel import boundaries valid");
  }
}
