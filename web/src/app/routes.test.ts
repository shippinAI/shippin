// The route folders are the only place pages and navigation entries are declared (docs/web-framework.md).
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { matchRoutes } from "../lib/router/match";
import type { NavSource } from "../lib/router/nav";
import { navSources as consoleNav } from "./nav.console.gen";
import { navSources as adminNav } from "./nav.admin.gen";
import { navSources as opsNav } from "./nav.ops.gen";
import { adminGroups } from "../template/admin/admin-nav";
import { accountNav, infrastructureNav, operatorNav, tenantNav } from "../template/nav-items";

const app = fileURLToPath(new URL(".", import.meta.url));
const src = join(app, "..");
let failures = 0;
const fail = (msg: string) => {
  failures++;
  console.error("FAIL " + msg);
};

// The path patterns of a tree's pages, from its generated RoutePath.
function pages(tree: string): string[] {
  const text = readFileSync(join(app, `routes.${tree}.gen.ts`), "utf8");
  return [...text.slice(text.indexOf("export type RoutePath")).matchAll(/\| "([^"]+)"/g)].map((m) => m[1]);
}

// An address that a path pattern matches: each parameter given a value.
const sample = (pattern: string) => pattern.replace(/:[a-zA-Z]+/g, "p1").replace(/\/\*$/, "/rest");

function checkTree(tree: string, sources: NavSource[], sections: Record<string, number>, within = "") {
  const paths = pages(tree);
  const routes = paths.map((path) => ({ path }));
  const ids = new Set<string>();
  let entries = 0;
  for (const s of sources)
    for (const e of s.meta.nav ?? []) {
      entries++;
      if (ids.has(e.id)) fail(`${tree}: navigation id ${e.id} is declared twice`);
      ids.add(e.id);
      if (!(e.section in sections)) fail(`${tree}: ${e.id} is in section ${e.section}, which the navigation does not show`);
      if (e.soon) continue;
      if (!s.page) fail(`${tree}: ${e.id} is declared in ${s.path}, which has no page`);
      let link = e.to ?? s.path;
      if (!link.startsWith("/")) link = within + (link ? "/" + link : "");
      const hit = matchRoutes(routes, sample(link));
      if (!hit) fail(`${tree}: ${e.id} links to ${link}, which no page serves`);
      else if (e.to !== undefined && !matchRoutes([{ path: s.path }], sample(link)))
        fail(`${tree}: ${e.id} is declared in ${s.path} but links to ${link}, another page; declare it there`);
      const own = within && s.path.startsWith(within) ? s.path.slice(within.length) : s.path;
      if (e.to === undefined && /[:*]/.test(own)) fail(`${tree}: ${e.id} is on ${s.path}, a pattern; give it the address it links to`);
    }
  const listed = Object.values(sections).reduce((a, b) => a + b, 0);
  if (listed !== entries) fail(`${tree}: ${entries} navigation entries are declared and ${listed} are shown`);
  if (paths.length === 0) fail(`${tree}: no pages`);
}

checkTree(
  "console",
  consoleNav,
  { devspace: tenantNav.length, infrastructure: infrastructureNav.length, operator: operatorNav.length, account: accountNav.length },
  "/t/:tenantId",
);
checkTree("admin", adminNav, Object.fromEntries(adminGroups.map((g) => [g.id, g.items.length])));
checkTree("ops", opsNav, {});

// Nothing outside the route folders lists a page, a route or a navigation entry by hand.
function walk(dir: string, out: string[] = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (/\.tsx?$/.test(name) && !/\.(gen|test)\.ts$/.test(name)) out.push(p);
  }
  return out;
}
for (const file of [...walk(join(src, "app")), ...walk(join(src, "template"))]) {
  const text = readFileSync(file, "utf8");
  const rel = file.slice(src.length + 1).replace(/\\/g, "/");
  if (/\bpath:\s*["']\//.test(text)) fail(`${rel} lists a route; add a folder under src/routes instead`);
  if (/lazy\(\(\) => import\(["'][^"']*-page["']\)/.test(text)) fail(`${rel} loads a page itself; add a folder under src/routes instead`);
  if (/\bicon:\s*[A-Z]\w*\s*,/.test(text)) fail(`${rel} lists a navigation entry; declare it in its page's meta.ts`);
}

// The tests find the generated files current: npm test runs `node scripts/routes.mjs --check` first.
if (failures) {
  console.error(`${failures} route check(s) failed`);
  process.exit(1);
}
console.log("routes: every navigation entry has its page, and nothing lists routes by hand");
