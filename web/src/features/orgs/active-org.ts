import type { OrgItem } from "../../lib/api/org-types";

/** The active org follows the URL: /ops/* is Operations, /t/<tenant>/* is that customer org. */
export function activeOrg(items: OrgItem[], path: string): OrgItem | undefined {
  if (path.startsWith("/ops")) return items.find((o) => o.type === "operations");
  const tid = /^\/t\/([^/]+)/.exec(path)?.[1];
  return items.find((o) => o.tenant_id && o.tenant_id === tid) ?? items.find((o) => o.type === "customer");
}

/** Where switching to an org lands. */
export function orgHome(o: OrgItem): string {
  return o.type === "operations" ? "/ops/servers" : `/t/${o.tenant_id}`;
}
