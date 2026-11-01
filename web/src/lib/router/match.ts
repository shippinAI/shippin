import type { ReactNode } from "react";

/** One route: nested, pathless (layout), index, `:param` and `*` segments; absolute or relative paths. */
export type RouteObject = { path?: string; index?: boolean; element?: ReactNode; children?: RouteObject[] };
export type Match = { route: RouteObject; params: Record<string, string> };

const split = (p: string) => p.split("/").filter(Boolean);

/** The matched branch (outermost first) for pathname, or null. First match in order wins. */
export function matchRoutes(routes: RouteObject[], pathname: string): Match[] | null {
  return matchList(routes, split(pathname).map(decodeURIComponent), 0);
}

function matchList(routes: RouteObject[], segs: string[], pos: number): Match[] | null {
  for (const r of routes) {
    const m = matchOne(r, segs, pos);
    if (m) return m;
  }
  return null;
}

function matchOne(r: RouteObject, segs: string[], pos: number): Match[] | null {
  if (r.index) return pos === segs.length ? [{ route: r, params: {} }] : null;
  const params: Record<string, string> = {};
  let at = pos;
  if (r.path !== undefined) {
    const pat = split(r.path);
    at = r.path.startsWith("/") ? 0 : pos;
    for (const p of pat) {
      if (p === "*") {
        params["*"] = segs.slice(at).join("/");
        at = segs.length;
        break;
      }
      if (at >= segs.length) return null;
      if (p.startsWith(":")) params[p.slice(1)] = segs[at];
      else if (p !== segs[at]) return null;
      at++;
    }
    if (at < pos) return null;
  }
  if (r.children) {
    const sub = matchList(r.children, segs, at);
    if (sub) return [{ route: r, params }, ...sub];
  }
  return at === segs.length && (r.path !== undefined || !r.children) ? [{ route: r, params }] : null;
}
