import type { CoverageTier } from "../../lib/api/catalog-types";

// How coverage reads in the panel, shared by the operator's catalog and the customer's own view.

export const supportHoursLabel: Record<CoverageTier["support_hours"], string> = {
  business: "Business hours",
  critical_always: "24/7 for outages, business hours for the rest",
  always: "24/7",
};

export const operationsLabel: Record<CoverageTier["operations"], string> = {
  self: "You run your workloads; the platform is run for you",
  assisted: "Guidance on your changes and checks of your setup",
  managed: "Operated for you remotely: monitoring, updates, backups, incidents and changes",
};

export const reviewsLabel: Record<CoverageTier["reviews"], string> = { none: "None", quarterly: "Quarterly", monthly: "Monthly" };

export const termLabel: Record<CoverageTier["consulting"], string> = { none: "Not offered", quoted: "On request, quoted", included: "Included" };

export const channelLabel: Record<CoverageTier["channels"][number], string> = {
  docs: "Documentation", community: "Community", email: "Email", chat: "Chat", phone: "Phone", video: "Video calls",
};

export const signInLabel: Record<CoverageTier["sign_in"], string> = {
  password: "Password or better",
  second_factor: "Passkey or identity provider",
  identity_provider: "Identity provider only",
};

export const severities = [
  { key: "critical_minutes", label: "Outage", hint: "Production is down" },
  { key: "high_minutes", label: "Degraded", hint: "Production works, badly" },
  { key: "normal_minutes", label: "Question", hint: "Anything else" },
] as const;

/** Whether a severity's target counts every hour or only business hours (8 a day). */
export function clockFor(hours: CoverageTier["support_hours"], key: (typeof severities)[number]["key"]): "always" | "business" {
  if (hours === "always") return "always";
  return hours === "critical_always" && key === "critical_minutes" ? "always" : "business";
}

const plural = (n: number, unit: string) => `${n} ${unit}${n === 1 ? "" : "s"}`;

/** "15 minutes", "4 hours", "2 business days": a first-response target in words. */
export function target(minutes: number | null | undefined, clock: "always" | "business"): string {
  if (!minutes) return "No target";
  if (minutes < 60) return plural(minutes, "minute");
  if (clock === "business" && minutes % 480 === 0) return plural(minutes / 480, "business day");
  const h = minutes / 60;
  const hours = Number.isInteger(h) ? plural(h, "hour") : `${Math.floor(h)} h ${minutes % 60} min`;
  return clock === "business" ? hours.replace(/hours?$/, (u) => `business ${u}`) : hours;
}

/** Professional services a tier offers, each none, quoted or included. */
export const services = [
  { key: "consulting", label: "Consulting" },
  { key: "custom_deployments", label: "Custom deployments" },
  { key: "remote_operations", label: "Remote operations" },
  { key: "team_training", label: "Team training" },
] as const;
