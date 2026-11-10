// Enforces docs/design-system.md over web/src:
//   every class in a class list resolves under the strict token theme in src/styles/tokens.css;
//   arbitrary bracket values only for structure (grid templates, transition lists, viewport sizes, React Aria variables);
//   no decorative effects (blur, gradients, glass);
//   no colour literals outside the token file, and no literal colours or pixels in inline styles.
// A line may opt out of the colour rule with a `tokens-allow: <reason>` comment.
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath, pathToFileURL } from "node:url";

const web = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const require = createRequire(path.join(web, "package.json"));
const { parseSync } = await import("vite"); // Rolldown's Oxc parser: TypeScript and JSX to ESTree
const { __unstable__loadDesignSystem: loadDesignSystem } = require("tailwindcss");
const styles = path.join(web, "src/styles");

async function designSystem(css) {
  return loadDesignSystem(css, {
    base: styles,
    async loadStylesheet(id, base) {
      const file = id === "tailwindcss" ? require.resolve("tailwindcss/index.css") : path.resolve(base, id);
      return { path: file, base: path.dirname(file), content: fs.readFileSync(file, "utf8") };
    },
    async loadModule(id, base) {
      const file = id.startsWith(".") ? path.resolve(base, id) : require.resolve(id);
      return { path: file, base: path.dirname(file), module: (await import(pathToFileURL(file).href)).default };
    },
  });
}

const app = fs.readFileSync(path.join(styles, "app.css"), "utf8");
const tokens = fs.readFileSync(path.join(styles, "tokens.css"), "utf8");
const strict = await designSystem(app);
// The same tokens on top of Tailwind's defaults: a class valid here but not in `strict` uses a value outside the tokens.
const loose = await designSystem(app.replace('@import "./tokens.css";', tokens.replace(/^\s*--[a-z-]+-\*: initial;$/gm, "")));

const memo = new Map();
const resolves = (ds, tag, c) => {
  const key = tag + c;
  if (!memo.has(key)) memo.set(key, ds.candidatesToCss([c])[0] != null);
  return memo.get(key);
};
const markers = new Set(["group", "peer"]);
const classShaped = /^!?-?[a-z@[][^\s"'`]*$/;
const decorative = /^(backdrop-|bg-(gradient|linear|radial|conic)|from-|via-|to-|blur|glass$|grid-bg$|drop-shadow|text-shadow)/;
const structural = /^(grid-cols|grid-rows|col-span|row-span|transition|\[[a-z-]+:)/;
const viewport = /^(\d+(\.\d+)?(d|s|l)?(vh|vw)|calc\(100(d|s|l)?vh-\d+(\.\d+)?rem\)|var\(--trigger-(width|anchor-point)\))$/;
const colourCall = /\b(oklch|oklab|rgba?|hsla?|lab|lch|hwb|color)\(/i;
const hexOnly = /^#[0-9a-f]{3,8}$/i;
const lengthLiteral = /^-?\d+(\.\d+)?(px|rem|em)$/;

/** Splits "md:data-[x=y]:p-4" into variants and the utility, respecting brackets. */
function utilityOf(candidate) {
  let depth = 0, cut = -1;
  for (let i = 0; i < candidate.length; i++) {
    const ch = candidate[i];
    if (ch === "[" || ch === "(") depth++;
    else if (ch === "]" || ch === ")") depth--;
    else if (ch === ":" && depth === 0) cut = i;
  }
  return candidate.slice(cut + 1).replace(/^!|!$/g, "");
}

function checkClassList(text, report) {
  const parts = text.split(/\s+/).filter(Boolean);
  if (!parts.length || !parts.every((p) => classShaped.test(p))) return;
  if (parts.length === 1 && !/[-:[]/.test(parts[0])) return; // a bare word such as "shadow" is as often data as a class
  if (!parts.some((p) => markers.has(p) || resolves(loose, "l", p))) return; // prose, ids, keys
  for (const p of parts) {
    if (markers.has(p) || /^(group|peer)\//.test(p)) continue;
    const util = utilityOf(p).replace(/^-/, "");
    if (decorative.test(util)) { report(p, "decorative effect; the design is flat"); continue; }
    const bracket = util.match(/^([a-z0-9-]+?)-\[(.+)\]$/) || util.match(/^\[(.+)\]$/);
    if (bracket) {
      const value = bracket.length === 3 ? bracket[2] : bracket[1];
      if (!(structural.test(util) || viewport.test(value))) { report(p, "arbitrary value; use a token"); continue; }
    }
    if (/\/\[/.test(util)) { report(p, "arbitrary opacity; use a whole-number modifier such as /10"); continue; }
    if (resolves(strict, "s", p)) continue;
    report(p, resolves(loose, "l", p) ? "uses a value outside the tokens" : "draws nothing");
  }
}

const files = [];
(function walk(dir) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) walk(p);
    else if (/\.(tsx?|css)$/.test(e.name) && !/\.test\.tsx?$/.test(e.name)) files.push(p);
  }
})(path.join(web, "src"));

const problems = [];
for (const file of files) {
  const rel = path.relative(web, file).split(path.sep).join("/");
  const src = fs.readFileSync(file, "utf8");
  const lines = src.split("\n");
  const allowed = (line) => /tokens-allow:\s*\S/.test(lines[line] ?? "") || /tokens-allow:\s*\S/.test(lines[line - 1] ?? "");
  if (file.endsWith(".css")) {
    if (rel === "src/styles/tokens.css") continue;
    lines.forEach((l, i) => {
      if ((/#[0-9a-f]{3,8}\b/i.test(l) || /\b(oklch|oklab|rgba?|hsla?)\(/i.test(l)) && !allowed(i)) problems.push(`${rel}:${i + 1}  colour literal outside the token file`);
    });
    continue;
  }
  const parsed = parseSync(file, src);
  if (parsed.errors?.length) { problems.push(`${rel}  could not be parsed: ${parsed.errors[0].message}`); continue; }
  const starts = [0];
  for (let i = 0; i < src.length; i++) if (src[i] === "\n") starts.push(i + 1);
  const lineOf = (offset) => { let lo = 0, hi = starts.length - 1; while (lo < hi) { const mid = (lo + hi + 1) >> 1; if (starts[mid] <= offset) lo = mid; else hi = mid - 1; } return lo; };
  const keyName = (k) => k?.name ?? k?.value;
  const visit = (node) => {
    if (!node || typeof node !== "object") return;
    if (Array.isArray(node)) { node.forEach(visit); return; }
    let text;
    if (node.type === "Literal" && typeof node.value === "string") text = node.value;
    else if (node.type === "TemplateElement") text = node.value?.cooked ?? node.value?.raw;
    if (text !== undefined) {
      const line = lineOf(node.start);
      const report = (c, why) => problems.push(`${rel}:${line + 1}  ${c}  ${why}`);
      if ((hexOnly.test(text.trim()) || colourCall.test(text)) && !allowed(line)) report(JSON.stringify(text.slice(0, 40)), "colour literal; use a token");
      else checkClassList(text, report);
    }
    if (node.type === "JSXAttribute" && keyName(node.name) === "style" && node.value?.expression?.type === "ObjectExpression") {
      for (const prop of node.value.expression.properties) {
        if (prop.type !== "Property") continue;
        const v = prop.value;
        const literal = v.type !== "Literal" ? false
          : typeof v.value === "number" ? v.value !== 0 && String(v.value)
          : typeof v.value === "string" && (lengthLiteral.test(v.value) || hexOnly.test(v.value) || colourCall.test(v.value)) && v.value;
        if (literal) problems.push(`${rel}:${lineOf(prop.start) + 1}  style ${keyName(prop.key)}: ${literal}  inline literal; use a token class`);
      }
    }
    for (const key in node) if (key !== "parent" && node[key] && typeof node[key] === "object") visit(node[key]);
  };
  visit(parsed.program);
}

if (problems.length) {
  console.error(`Design token check: ${problems.length} problem(s). See docs/design-system.md.\n  ${problems.join("\n  ")}`);
  process.exit(1);
}
console.log(`Design token check: ${files.length} files follow the tokens.`);
