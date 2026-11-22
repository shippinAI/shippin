import { navSources } from "../../app/nav.admin.gen";
import { navSection } from "../../lib/router/nav";
import type { NavItem, NavSection } from "../nav-items";

export type AdminGroup = NavSection & { items: NavItem[] };

/** The provider panel's sections, grouped by what they are for. Each entry is declared in the meta.ts beside
 *  its page (src/routes/admin); cap is the staff permission that shows it. */
const sections: NavSection[] = [
  { id: "admin.start", label: "Start" },
  { id: "admin.infrastructure", label: "Infrastructure" },
  { id: "admin.platform", label: "Platform" },
  { id: "admin.people", label: "People" },
  { id: "admin.customers", label: "Customers" },
  { id: "admin.developer", label: "Developer" },
];

export const adminGroups: AdminGroup[] = sections.map((s) => ({ ...s, items: navSection(navSources, s.id) }));

export const adminNav: NavItem[] = adminGroups.flatMap((g) => g.items);
