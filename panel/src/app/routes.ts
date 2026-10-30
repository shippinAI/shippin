export type AppMode = "customer" | "operator";

export type ConsoleRoute = {
  id: string;
  label: string;
  mode: AppMode;
  tags: string[];
};

export const CONSOLE_ROUTES: ConsoleRoute[] = [
  { id: "cust-home", label: "Home", mode: "customer", tags: ["me"] },
  {
    id: "cust-workspaces",
    label: "Workspaces",
    mode: "customer",
    tags: ["folders", "workspaces", "jobs"]
  },
  {
    id: "cust-team",
    label: "Team",
    mode: "customer",
    tags: ["members", "settings"]
  },
  {
    id: "cust-secrets",
    label: "Secrets",
    mode: "customer",
    tags: ["secrets"]
  },
  {
    id: "cust-identity-ai",
    label: "Identity + AI",
    mode: "customer",
    tags: ["identity", "rrflow"]
  },
  {
    id: "cust-audit",
    label: "Audit",
    mode: "customer",
    tags: ["audit"]
  },
  {
    id: "op-home",
    label: "Operator Home",
    mode: "operator",
    tags: ["me", "platform"]
  },
  {
    id: "op-tenants",
    label: "Tenants",
    mode: "operator",
    tags: ["tenants", "jobs"]
  },
  {
    id: "op-settings",
    label: "Platform Settings",
    mode: "operator",
    tags: ["settings"]
  },
  {
    id: "op-audit",
    label: "Platform Audit",
    mode: "operator",
    tags: ["audit"]
  }
];
