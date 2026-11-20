#!/usr/bin/env node
// Writes src/app/routes.<tree>.gen.ts and nav.<tree>.gen.ts from the folders under src/routes/<tree>/ and
// src/modules/<name>/routes/<tree>/ (docs/web-framework.md). No route or navigation entry is listed by
// hand anywhere else.
//
//   page.tsx    a page at the folder's path (default export)
//   layout.tsx  wraps every page below the folder (default export, renders <Outlet />)
//   meta.ts     optional: the folder's navigation entries; `eager: true` loads its layout with the app
//   mounts.ts   optional: `export default ["<tree>"]` places that tree's routes here; the folder's path
//               must be the mounted tree's own
//   [name]/     a path parameter (:name);  [...name]/ the rest of the path (*);  (name)/ a group, no segment
//
// Each tree is its own file, so one app's bundle never carries another's route table.
// Usage: node scripts/routes.mjs [--check] [--src <dir>]
import { existsSync, readdirSync, readFileSync, statSync, unlinkSync, writeFileSync } from "node:fs";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

/** Reads the route folders under src and returns each tree's file text, by file name. */
export function generate(src) {
  const problems = [];
  const roots = [];
  const routesDir = join(src, "routes");
  if (existsSync(routesDir)) for (const t of dirs(routesDir)) roots.push({ tree: t, dir: join(routesDir, t), owner: "the panel" });
  const modulesDir = join(src, "modules");
  if (existsSync(modulesDir))
    for (const m of dirs(modulesDir)) {
      const r = join(modulesDir, m, "routes");
      if (existsSync(r)) for (const t of dirs(r)) roots.push({ tree: t, dir: join(r, t), owner: `module ${m}` });
    }

  const trees = new Map();
  for (const r of roots) {
    if (!/^[a-z][a-z0-9-]*$/.test(r.tree)) {
      problems.push(`${r.dir}: a route tree is one lower-case word`);
      continue;
    }
    if (!trees.has(r.tree)) trees.set(r.tree, node(""));
    merge(trees.get(r.tree), r.dir, r.owner, problems);
  }

  const files = new Map();
  for (const [tree, root] of [...trees].sort()) {
    for (const m of mountsOf(root)) {
      if (!trees.has(m)) problems.push(`tree ${tree} mounts ${m}, which does not exist`);
      else if (m === tree || mountsOf(trees.get(m)).length) problems.push(`tree ${tree} mounts ${m}, which must be a tree that mounts nothing`);
    }
    const { routes, nav } = emit(src, tree, root, problems);
    files.set(`routes.${tree}.gen.ts`, routes);
    files.set(`nav.${tree}.gen.ts`, nav);
  }
  return { files, problems };
}

function emit(src, tree, root, problems) {
  const imports = [];
  const navImports = [];
  const nav = [];
  const seen = new Map();
  const mounted = new Set();
  const ids = new Map();
  const id = (file) => {
    if (!ids.has(file)) ids.set(file, `m${ids.size}`);
    return ids.get(file);
  };
  const rel = (file) => {
    const p = relative(join(src, "app"), file).split(sep).join("/").replace(/\.tsx?$/, "");
    return p.startsWith(".") ? p : "./" + p;
  };
  // A page or layout file that only re-exports another module's component is loaded from that module
  // directly, so the bundle has no extra chunk for it.
  const target = (file) => {
    const m = /^export \{ (\w+) as default \} from "(\.[^"]+)";\s*$/.exec(readFileSync(file, "utf8"));
    return m ? { name: m[1], spec: rel(join(dirname(file), m[2])) } : null;
  };
  const load = (file) => {
    const t = target(file);
    return t ? `() => import(${JSON.stringify(t.spec)}).then((m) => ({ default: m.${t.name} }))` : `() => import(${JSON.stringify(rel(file))})`;
  };
  const eagerImport = (v, file) => {
    const t = target(file);
    return t ? `import { ${t.name} as ${v} } from ${JSON.stringify(t.spec)};` : `import ${v} from ${JSON.stringify(rel(file))};`;
  };

  // The routes a folder contributes to its nearest layout: its page, any mounted tree, then its children's.
  function build(n, url) {
    const here = n.seg === undefined ? url : (url === "/" ? "" : url) + "/" + n.seg;
    const items = [];
    if (n.page) {
      const p = here || "/";
      if (seen.has(p)) problems.push(`${tree}: ${p} is served by both ${seen.get(p)} and ${n.page.owner}`);
      else seen.set(p, n.page.owner);
      items.push({ sort: p, code: `{ path: ${JSON.stringify(p)}, page: ${load(n.page.file)} }` });
    }
    if (n.meta) {
      const v = id(n.meta.file);
      navImports.push(`import ${v} from ${JSON.stringify(rel(n.meta.file))};`);
      nav.push(`{ path: ${JSON.stringify(here || "/")}, page: ${Boolean(n.page)}, meta: ${v} }`);
    }
    for (const m of n.mounts) {
      mounted.add(m);
      items.push({ sort: (here || "") + "/~mount", code: `...${mountVar(m)}` });
    }
    for (const c of [...n.children.values()].sort(bySegment)) items.push(...build(c, here));
    if (!n.layout) return items;
    items.sort((a, b) => byPath(a.sort, b.sort));
    let layout;
    if (n.eager) {
      const v = id(n.layout.file);
      imports.push(eagerImport(v, n.layout.file));
      layout = `layout: ${v}`;
    } else layout = `load: ${load(n.layout.file)}`;
    const path = n.seg !== undefined ? `path: ${JSON.stringify(here)}, ` : "";
    // A group's layout is tried after its siblings, so a page beside it is never shadowed by one inside it.
    const sort = n.seg === undefined ? (here || "") + "/~group" : here;
    return [{ sort, code: `{ ${path}${layout}, children: [\n${indent(items.map((i) => i.code).join(",\n"))},\n] }` }];
  }

  const items = build(root, "").sort((a, b) => byPath(a.sort, b.sort));
  const mountImports = [...mounted].sort().map((m) => `import { routes as ${mountVar(m)} } from "./routes.${m}.gen";`);
  const paths = [...seen.keys()].sort();
  const head = `// Generated by scripts/routes.mjs from the folders under src/routes/${tree} and src/modules/*/routes/${tree}.
// Do not edit: run \`npm run routes\` after adding, moving or removing a page, layout, meta.ts or mounts.ts.
`;
  const routes = `${head}import type { GenRoute } from "../lib/router/tree";
${[...mountImports, ...new Set(imports)].join("\n")}

/** The ${tree} app's routes. */
export const routes: GenRoute[] = [
${indent(items.map((i) => i.code).join(",\n"))},
];

/** The path pattern of every page of the ${tree} app. */
export type RoutePath =${paths.length ? paths.map((p) => `\n  | ${JSON.stringify(p)}`).join("") : " never"};
`;
  // The navigation entries are a file of their own, so the code that starts the app does not carry them.
  const navText = `${head}import type { NavSource } from "../lib/router/nav";
${[...new Set(navImports)].join("\n")}

/** Every folder of the ${tree} app with a meta.ts, at its path, and whether it has a page. */
export const navSources: NavSource[] = [
${indent(nav.join(",\n"))}${nav.length ? "," : ""}
];
`;
  return { routes, nav: navText };
}

const indent = (s) =>
  s
    .split("\n")
    .map((l) => (l ? "  " + l : l))
    .join("\n");

const mountVar = (t) => "mount_" + t.replace(/[^a-zA-Z0-9]/g, "_");

function dirs(d) {
  return readdirSync(d)
    .filter((n) => statSync(join(d, n)).isDirectory())
    .sort();
}

function node(name) {
  return { name, seg: segment(name), children: new Map(), mounts: [] };
}

// The URL segment a folder adds: undefined for the tree's root and for a group.
function segment(name) {
  if (name === "" || /^\(.+\)$/.test(name)) return undefined;
  if (/^\[\.\.\.[a-zA-Z][a-zA-Z0-9]*\]$/.test(name)) return "*";
  const param = /^\[([a-zA-Z][a-zA-Z0-9]*)\]$/.exec(name);
  return param ? ":" + param[1] : name;
}

const kinds = { "page.tsx": "page", "layout.tsx": "layout", "meta.ts": "meta", "mounts.ts": "mounts" };

function merge(n, dir, owner, problems) {
  for (const name of readdirSync(dir).sort()) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) {
      if (!/^(\([a-z][a-z0-9-]*\)|\[(\.\.\.)?[a-zA-Z][a-zA-Z0-9]*\]|[a-z0-9][a-z0-9.-]*)$/.test(name)) {
        problems.push(`${p}: a route folder is lower-case words, [param], [...rest] or (group)`);
        continue;
      }
      if (!n.children.has(name)) n.children.set(name, node(name));
      merge(n.children.get(name), p, owner, problems);
      continue;
    }
    const kind = kinds[name];
    if (!kind) continue; // a page may keep its own parts beside it
    if (kind === "mounts") {
      for (const m of readFileSync(p, "utf8").matchAll(/"([a-z][a-z0-9-]*)"/g)) n.mounts.push(m[1]);
      continue;
    }
    if (n[kind]) {
      problems.push(`${p}: ${owner} adds a ${kind} where ${n[kind].owner} already has one`);
      continue;
    }
    n[kind] = { file: p, owner };
    if (kind === "meta" && /\beager:\s*true\b/.test(readFileSync(p, "utf8"))) n.eager = true;
  }
}

function mountsOf(n) {
  return [...n.mounts, ...[...n.children.values()].flatMap(mountsOf)];
}

// Folders in matching order: static names, then parameters, then the rest of the path, then groups.
function rank(name) {
  if (name.startsWith("(")) return 3;
  if (name.startsWith("[...")) return 2;
  if (name.startsWith("[")) return 1;
  return 0;
}

function bySegment(a, b) {
  return rank(a.name) - rank(b.name) || a.name.localeCompare(b.name);
}

// Sibling routes in matching order: the most specific path first, mounted trees and groups last.
function byPath(a, b) {
  const sa = a.split("/").filter(Boolean);
  const sb = b.split("/").filter(Boolean);
  for (let i = 0; i < Math.max(sa.length, sb.length); i++) {
    if (sa[i] === undefined) return -1;
    if (sb[i] === undefined) return 1;
    const d = segRank(sa[i]) - segRank(sb[i]);
    if (d) return d;
    if (sa[i] !== sb[i]) return sa[i] < sb[i] ? -1 : 1;
  }
  return 0;
}

function segRank(s) {
  if (s === "~group") return 4;
  if (s === "~mount") return 3;
  if (s === "*") return 2;
  if (s.startsWith(":")) return 1;
  return 0;
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  const args = process.argv.slice(2);
  const check = args.includes("--check");
  const at = args.indexOf("--src");
  const src = at >= 0 ? args[at + 1] : fileURLToPath(new URL("../src", import.meta.url));
  const { files, problems } = generate(src);
  if (problems.length) {
    console.error(problems.map((p) => "routes: " + p).join("\n"));
    process.exit(1);
  }
  const app = join(src, "app");
  const stale = [];
  for (const [name, text] of files) {
    const file = join(app, name);
    const had = existsSync(file) ? readFileSync(file, "utf8") : "";
    if (had === text) continue;
    if (check) stale.push(name);
    else writeFileSync(file, text);
  }
  for (const name of readdirSync(app).filter((n) => /^(routes|nav)\.[a-z0-9-]+\.gen\.ts$/.test(n) && !files.has(n))) {
    if (check) stale.push(name);
    else unlinkSync(join(app, name));
  }
  if (check && stale.length) {
    console.error(`routes: ${stale.join(", ")} out of date with the folders under src/routes; run \`npm run routes\`.`);
    process.exit(1);
  }
  console.log(check ? "routes: up to date" : `routes: ${files.size / 2} trees`);
}
