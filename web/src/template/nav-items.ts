import { Bot, BarChart3, Globe, SquareTerminal, FolderGit2, KeySquare, Boxes, Fingerprint, Gauge, KeyRound, LayoutGrid, Network, Server, Settings, ShieldCheck, Users, Cable, Database, Cpu } from "lucide-react";
import type { LucideIcon } from "lucide-react";

export type NavItem = {
  label: string;
  icon: LucideIcon;
  /** Path under /t/:tenantId, or absolute for operator routes. */
  to?: string;
  hue: string;
  soon?: string;
  /** Only shown when /v1/me grants this capability. */
  cap?: string;
};

// Order and names follow CONSOLE-IA.md "Navigation tree per role".
export const tenantNav: NavItem[] = [
  { label: "Home", icon: LayoutGrid, to: "", hue: "var(--color-primary)" },
  { label: "Projects", icon: FolderGit2, to: "projects", hue: "var(--color-hue-workspaces)" },
  { label: "Database", icon: Database, to: "database", hue: "var(--color-hue-data)" },
  { label: "Workspaces", icon: Boxes, to: "workspaces", hue: "var(--color-hue-workspaces)" },
  { label: "Terminal", icon: SquareTerminal, to: "go/terminal", hue: "var(--color-hue-workspaces)" },
  { label: "Connect accounts", icon: KeyRound, to: "go/connect", hue: "var(--color-hue-secrets)" },
  { label: "Publish", icon: Globe, to: "go/publish", hue: "var(--color-hue-edge)" },
  { label: "Usage", icon: BarChart3, to: "go/usage", hue: "var(--color-hue-data)" },
  { label: "Secrets", icon: KeyRound, to: "/vaultix", hue: "var(--color-hue-secrets)" },
  { label: "Servers", icon: Server, to: "servers", hue: "var(--color-hue-edge)" },
  { label: "Identity", icon: Fingerprint, to: "identity", hue: "var(--color-hue-identity)" },
  { label: "Network", icon: Network, hue: "var(--color-hue-network)", soon: "Per-org network policy lands with hypervisor pools" },
  { label: "AI & Agents", icon: Bot, to: "go/ai", hue: "var(--color-hue-ai)" },
  { label: "Team", icon: Users, to: "team", hue: "var(--color-hue-billing)" },
  { label: "Settings", icon: Settings, to: "settings", hue: "var(--color-fg-muted)" },
];

export const operatorNav: NavItem[] = [
  { label: "Platform health", icon: Gauge, to: "/operator", hue: "var(--color-hue-data)" },
  { label: "Instances", icon: Server, to: "/operator/instances", hue: "var(--color-hue-edge)" },
  { label: "Hosts", icon: Cpu, to: "/operator/hosts", hue: "var(--color-hue-edge)", cap: "infra.hosts" },
  { label: "Provider panel", icon: ShieldCheck, to: "/admin", hue: "var(--color-warning)", cap: "provider.panel" },
  { label: "Governance", icon: ShieldCheck, hue: "var(--color-hue-ai)", soon: "Platform settings land in v0.2" },
];

export const accountNav: NavItem[] = [
  { label: "Agent hub", icon: LayoutGrid, to: "/hub", hue: "var(--color-hue-ai)" },
  { label: "Gold approvals", icon: ShieldCheck, to: "/gold", hue: "var(--color-warning)" },
  { label: "SSH keys", icon: KeySquare, to: "/account/ssh-keys", hue: "var(--color-hue-secrets)" },
  { label: "Automaton chat", icon: Bot, to: "/chat", hue: "var(--color-hue-ai)" },
  { label: "AI subscriptions", icon: Bot, to: "/account/ai", hue: "var(--color-hue-ai)" },
  { label: "Bridge sessions", icon: Cable, to: "/account/bridge", hue: "var(--color-hue-secrets)" },
];
