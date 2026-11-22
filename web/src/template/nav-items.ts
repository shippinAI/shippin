import { navSources } from "../app/nav.console.gen";
import { navSection } from "../lib/router/nav";
import type { NavItem } from "../lib/router/meta";

export type { NavItem } from "../lib/router/meta";

export type NavSection = { id: string; label: string };

// Each entry is declared in the meta.ts beside its page (src/routes/console, docs/web-framework.md);
// the order and names follow CONSOLE-IA.md "Navigation tree per role". Organization pages link relative
// to /t/:tenantId.
const organization = "/t/:tenantId";
export const tenantNav: NavItem[] = navSection(navSources, "devspace", organization);
export const infrastructureNav: NavItem[] = navSection(navSources, "infrastructure", organization);
export const operatorNav: NavItem[] = navSection(navSources, "operator", organization);
export const accountNav: NavItem[] = navSection(navSources, "account", organization);

export const navSections = {
  devspace: { id: "devspace", label: "Organization" },
  account: { id: "account", label: "Account" },
  infrastructure: { id: "infrastructure", label: "Infrastructure" },
  operator: { id: "operator", label: "Operator" },
} satisfies Record<string, NavSection>;

/** Whether this installation has the entry's feature switched on. */
export function featureOn(caps: Record<string, boolean>) {
  return (i: NavItem) => !i.feature || caps[`feature.${i.feature}`] === true;
}

/** Filters entries down to what /v1/me lets this caller open. */
export function allowedBy(caps: Record<string, boolean>) {
  return (i: NavItem) => (!i.cap || caps[i.cap]) && !(i.unlessCap && caps[i.unlessCap]) && featureOn(caps)(i);
}
