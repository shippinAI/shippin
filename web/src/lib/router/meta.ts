import type { LucideIcon } from "lucide-react";

/** One navigation entry as the sidebar shows it. */
export type NavItem = {
  /** Stable name; `ui.nav.items` renames, hides or moves an entry by it. */
  id: string;
  label: string;
  icon: LucideIcon;
  /** Path under /t/:tenantId, or absolute for routes outside an organization. */
  to?: string;
  hue: string;
  soon?: string;
  /** Only shown when this installation has switched this feature on. */
  feature?: string;
  /** Only shown when /v1/me grants this capability. */
  cap?: string;
  /** Hidden when /v1/me grants this capability: the caller has a better place for it. */
  unlessCap?: string;
  /** Also active on routes under this one. */
  prefix?: boolean;
  /** Listed in the phone menu. */
  mobile?: boolean;
};

/** A navigation entry declared beside its page, in the page folder's meta.ts. */
export type NavEntry = Omit<NavItem, "to"> & {
  /** The navigation section it is listed in. */
  section: string;
  /** Its place in the section; lower comes first. */
  order: number;
  /** Where it links when that is not the page's own path, such as one value of a [param] folder. */
  to?: string;
};

/** What a route folder's meta.ts declares (docs/web-framework.md). */
export type PageMeta = {
  /** Load this folder's layout with the app instead of on demand (the root layout and the sign-in gate). */
  eager?: boolean;
  nav?: NavEntry[];
};
