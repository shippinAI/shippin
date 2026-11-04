import { useQuery } from "@tanstack/react-query";
import { apiGet } from "../api/client";
import type { OpsServerList, OrgList } from "../api/org-types";
import { useMe } from "./use-api";

/** Orgs for the header switcher, when this installation uses them. */
export const useOrgs = () => {
  const on = useMe().data?.capabilities["feature.orgs"] === true;
  return useQuery({ queryKey: ["orgs"], queryFn: () => apiGet<OrgList>("/v1/orgs"), enabled: on, retry: false, staleTime: 60_000 });
};

/** Operations view of every server; staff only (403 for tenant roles). */
export const useOpsServers = () =>
  useQuery({ queryKey: ["ops", "servers"], queryFn: () => apiGet<OpsServerList>("/v1/ops/servers"), retry: false, refetchInterval: 15_000 });
