import { Blocks, Boxes, Building2, Contact, Layers, Fingerprint, Globe, LayoutTemplate, Palette, Server, ShieldCheck, SlidersHorizontal, UsersRound } from "lucide-react";
import type { NavItem } from "../nav-items";

/** Provider master panel sections (/admin); cap is the staff permission that shows the item. */
export const adminNav: NavItem[] = [
  { id: "admin.tenants", label: "Tenants", icon: Building2, to: "/admin/tenants", hue: "var(--color-primary)", cap: "tenants.read" },
  { id: "admin.customers", label: "Customers", icon: Contact, to: "/admin/customers", hue: "var(--color-primary)", cap: "customers.read" },
  { id: "admin.plans", label: "Plans & tiers", icon: Layers, to: "/admin/plans", hue: "var(--color-hue-ai)", cap: "catalog.plans.read" },
  { id: "admin.services", label: "Service catalog", icon: Blocks, to: "/admin/services", hue: "var(--color-hue-data)", cap: "catalog.plans.read" },
  { id: "admin.workspaces", label: "Workspaces", icon: Boxes, to: "/admin/workspaces", hue: "var(--color-hue-workspaces)", cap: "workspaces.read" },
  { id: "admin.access", label: "Access & audit", icon: ShieldCheck, to: "/admin/access", hue: "var(--color-danger)", cap: "audit.read|access.request|access.approve_breakglass" },
  { id: "admin.templates", label: "Templates", icon: LayoutTemplate, to: "/admin/templates", hue: "var(--color-hue-ai)", cap: "templates.read" },
  { id: "admin.identity", label: "Users & sign-in", icon: Fingerprint, to: "/admin/identity", hue: "var(--color-hue-identity)", cap: "identity.read" },
  { id: "admin.host", label: "Host", icon: Server, to: "/admin/host", hue: "var(--color-hue-data)", cap: "host.read" },
  { id: "admin.edge", label: "Edge", icon: Globe, to: "/admin/edge", hue: "var(--color-hue-edge)", cap: "edge.read" },
  { id: "admin.platform", label: "Platform settings", icon: SlidersHorizontal, to: "/admin/platform", hue: "var(--color-hue-data)", cap: "platform.settings.read" },
  { id: "admin.interface", label: "Interface", icon: Palette, to: "/admin/interface", hue: "var(--color-hue-data)", cap: "platform.settings.read" },
  { id: "admin.team", label: "Team", icon: UsersRound, to: "/admin/team", hue: "var(--color-warning)", cap: "team.read" },
];
