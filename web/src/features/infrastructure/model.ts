import type { InfraField, InfraOverview, InfraProvider } from "../../lib/hooks/use-infra";

export const OVERVIEW_TAB = "overview";

/** A provider gets a tab once a host is attached to it, so tabs always show something real. */
export const providersWithHosts = (o: InfraOverview | undefined): InfraProvider[] => (o?.providers ?? []).filter((p) => p.nodes.length > 0);

/** The tab a URL asks for, or the overview when that provider has no host to show. */
export function selectedTab(o: InfraOverview | undefined, wanted: string | undefined): string {
  return wanted && providersWithHosts(o).some((p) => p.kind === wanted) ? wanted : OVERVIEW_TAB;
}

export const tabHref = (id: string) => (id === OVERVIEW_TAB ? "/operator/hosts" : `/operator/hosts/${encodeURIComponent(id)}`);

export const slugFrom = (name: string) => name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 40).replace(/-+$/, "");

export const slugOK = (s: string) => /^[a-z0-9][a-z0-9-]{0,39}$/.test(s);
export const siteOK = (s: string) => /^[a-z0-9-]{1,40}$/.test(s);

export const initialValues = (fields: InfraField[]): Record<string, string> => Object.fromEntries(fields.map((f) => [f.name, f.default ?? ""]));

export const fieldsComplete = (fields: InfraField[], values: Record<string, string>) => fields.every((f) => !f.required || !!values[f.name]?.trim());

/** Only fields that have a value are sent, so a blank optional field is not stored as a setting. */
export const filledValues = (fields: InfraField[], values: Record<string, string>): Record<string, string> =>
  Object.fromEntries(fields.flatMap((f) => (values[f.name]?.trim() ? [[f.name, values[f.name]!.trim()]] : [])));

/** Status words come straight from each hypervisor and differ in case between them. */
export const isRunning = (status: string) => status.toLowerCase() === "running";
export const isStopped = (status: string) => status.toLowerCase() === "stopped";
