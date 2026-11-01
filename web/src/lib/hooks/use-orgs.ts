import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../api/client";
import type { OpsServerList, OrgList } from "../api/org-types";

/** Orgs for the header switcher. A 404 means the feature is off: the switcher hides. */
export const useOrgs = () =>
  useQuery({ queryKey: ["orgs"], queryFn: () => apiGet<OrgList>("/v1/orgs"), retry: false, staleTime: 60_000 });

/** Operations view of every server; staff only (403 for tenant roles). */
export const useOpsServers = () =>
  useQuery({ queryKey: ["ops", "servers"], queryFn: () => apiGet<OpsServerList>("/v1/ops/servers"), retry: false, refetchInterval: 15_000 });
