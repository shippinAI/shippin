import { Blocks, Boxes, Building2, Contact, Layers, Fingerprint, Globe, LayoutTemplate, Server, ShieldCheck, SlidersHorizontal, UsersRound } from "lucide-react";
import type { NavItem } from "../nav-items";

/** Provider master panel sections (/admin); cap is the staff permission that shows the item. */
export const adminNav: NavItem[] = [
  { label: "Tenants", icon: Building2, to: "/admin/tenants", hue: "var(--color-primary)", cap: "tenants.read" },
  { label: "Customers", icon: Contact, to: "/admin/customers", hue: "var(--color-primary)", cap: "customers.read" },
  { label: "Plans & tiers", icon: Layers, to: "/admin/plans", hue: "var(--color-hue-ai)", cap: "catalog.plans.read" },
  { label: "Service catalog", icon: Blocks, to: "/admin/services", hue: "var(--color-hue-data)", cap: "catalog.plans.read" },
  { label: "Workspaces", icon: Boxes, to: "/admin/workspaces", hue: "var(--color-hue-workspaces)", cap: "workspaces.read" },
  { label: "Access & audit", icon: ShieldCheck, to: "/admin/access", hue: "var(--color-danger)", cap: "audit.read|access.request|access.approve_breakglass" },
  { label: "Templates", icon: LayoutTemplate, to: "/admin/templates", hue: "var(--color-hue-ai)", cap: "templates.read" },
  { label: "Users & sign-in", icon: Fingerprint, to: "/admin/identity", hue: "var(--color-hue-identity)", cap: "identity.read" },
  { label: "Host", icon: Server, to: "/admin/host", hue: "var(--color-hue-data)", cap: "host.read" },
  { label: "Edge", icon: Globe, to: "/admin/edge", hue: "var(--color-hue-edge)", cap: "edge.read" },
  { label: "Platform settings", icon: SlidersHorizontal, to: "/admin/platform", hue: "var(--color-hue-data)", cap: "platform.settings.read" },
  { label: "Team", icon: UsersRound, to: "/admin/team", hue: "var(--color-warning)", cap: "team.read" },
];
