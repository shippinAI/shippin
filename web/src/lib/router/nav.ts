import type { NavEntry, NavItem, PageMeta } from "./meta";

/** A route folder with a meta.ts, at its path pattern, as scripts/routes.mjs writes it. */
export type NavSource = { path: string; page: boolean; meta: PageMeta };

/**
 * A section's navigation entries in order. An entry links to its page's own path unless it says
 * otherwise; within means "relative to this path" (such as /t/:tenantId), and an entry marked soon has
 * no link.
 */
export function navSection(sources: NavSource[], section: string, within = ""): NavItem[] {
  const entries: { e: NavEntry; path: string }[] = [];
  for (const s of sources) for (const e of s.meta.nav ?? []) if (e.section === section) entries.push({ e, path: s.path });
  entries.sort((a, b) => a.e.order - b.e.order);
  return entries.map(({ e, path }) => {
    const { section: _s, order: _o, to, ...item } = e;
    if (e.soon) return item;
    let link = to ?? path;
    if (to === undefined && within && (link === within || link.startsWith(within + "/"))) link = link.slice(within.length + 1);
    return { ...item, to: link };
  });
}
