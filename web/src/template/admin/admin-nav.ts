import { Blocks, Boxes, Braces, Building2, Contact, Cpu, Database, Layers, Fingerprint, Globe, LayoutTemplate, Palette, Rocket, Server, ShieldCheck, SlidersHorizontal, UsersRound } from "lucide-react";
import type { NavItem, NavSection } from "../nav-items";

export type AdminGroup = NavSection & { items: NavItem[] };

/** Every section of the console, grouped by what it is for. cap is the staff permission that shows the item. */
export const adminGroups: AdminGroup[] = [
  {
    id: "admin.start", label: "Start",
    items: [
      { id: "admin.get-started", label: "Get started", icon: Rocket, to: "/admin/start", hue: "var(--color-primary)", cap: "platform.settings.read" },
    ],
  },
  {
    id: "admin.infrastructure", label: "Infrastructure",
    items: [
      { id: "admin.infrastructure", label: "Hosts & guests", icon: Cpu, to: "/admin/infrastructure", hue: "var(--color-hue-edge)", cap: "infra.read", prefix: true },
      { id: "admin.host", label: "Host", icon: Server, to: "/admin/host", hue: "var(--color-hue-data)", cap: "host.read", feature: "workspaces" },
      { id: "admin.edge", label: "Edge", icon: Globe, to: "/admin/edge", hue: "var(--color-hue-edge)", cap: "edge.read", feature: "workspaces" },
    ],
  },
  {
    id: "admin.platform", label: "Platform",
    items: [
      { id: "admin.built-in", label: "Built-in services", icon: Database, to: "/admin/built-in", hue: "var(--color-hue-data)", cap: "platform.settings.read" },
      { id: "admin.platform", label: "Platform settings", icon: SlidersHorizontal, to: "/admin/platform", hue: "var(--color-hue-data)", cap: "platform.settings.read" },
      { id: "admin.interface", label: "Interface", icon: Palette, to: "/admin/interface", hue: "var(--color-hue-data)", cap: "platform.settings.read" },
    ],
  },
  {
    id: "admin.people", label: "People",
    items: [
      { id: "admin.team", label: "Team", icon: UsersRound, to: "/admin/team", hue: "var(--color-warning)", cap: "team.read" },
      { id: "admin.identity", label: "Users & sign-in", icon: Fingerprint, to: "/admin/identity", hue: "var(--color-hue-identity)", cap: "identity.read", feature: "idp_admin" },
      { id: "admin.access", label: "Access & audit", icon: ShieldCheck, to: "/admin/access", hue: "var(--color-danger)", cap: "audit.read|access.request|access.approve_breakglass" },
    ],
  },
  {
    id: "admin.customers", label: "Customers",
    items: [
      { id: "admin.tenants", label: "Tenants", icon: Building2, to: "/admin/tenants", hue: "var(--color-primary)", cap: "tenants.read" },
      { id: "admin.customers", label: "Customers", icon: Contact, to: "/admin/customers", hue: "var(--color-primary)", cap: "customers.read", feature: "catalog" },
      { id: "admin.plans", label: "Plans & tiers", icon: Layers, to: "/admin/plans", hue: "var(--color-hue-ai)", cap: "catalog.plans.read", feature: "catalog" },
      { id: "admin.services", label: "Service catalog", icon: Blocks, to: "/admin/services", hue: "var(--color-hue-data)", cap: "catalog.plans.read", feature: "catalog" },
      { id: "admin.workspaces", label: "Workspaces", icon: Boxes, to: "/admin/workspaces", hue: "var(--color-hue-workspaces)", cap: "workspaces.read", feature: "workspaces" },
      { id: "admin.templates", label: "Templates", icon: LayoutTemplate, to: "/admin/templates", hue: "var(--color-hue-ai)", cap: "templates.read", feature: "workspaces" },
    ],
  },
  {
    id: "admin.developer", label: "Developer",
    items: [
      { id: "admin.api", label: "API explorer", icon: Braces, to: "/admin/api", hue: "var(--color-hue-data)" },
    ],
  },
];

export const adminNav: NavItem[] = adminGroups.flatMap((g) => g.items);
