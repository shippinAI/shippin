import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { callAction } from "../api/actions";
import { apiGet, apiSend } from "../api/client";
import type { CatalogService, Items, Plan, PlanService } from "../api/catalog-types";
import { toast } from "../design/components/toast/toast-queue";

const base = "/v1/operator/catalog";
const get = <T,>(key: unknown[], path: string) => useQuery({ queryKey: ["catalog", ...key], queryFn: () => apiGet<T>(path), retry: false });

export const usePlans = () => get<Items<Plan>>(["plans"], `${base}/plans`);
export const useCoverageTiers = () =>
  useQuery({ queryKey: ["catalog", "coverage"], queryFn: () => callAction("shippin.tenancy.coverage_tier.list"), retry: false });
export const useServices = () => get<Items<CatalogService>>(["services"], `${base}/services`);
export const usePlanServices = () => get<Items<PlanService>>(["plan-services"], `${base}/plan-services`);
export const useCustomers = () =>
  useQuery({ queryKey: ["catalog", "customers"], queryFn: () => callAction("shippin.tenancy.customer.list"), retry: false });
export const useCustomer = (customerId: string) =>
  useQuery({ queryKey: ["catalog", "customers", customerId], queryFn: () => callAction("shippin.tenancy.customer.get", { customerId }), retry: false });
export const useEffective = (customerId: string) =>
  useQuery({ queryKey: ["catalog", "effective", customerId], queryFn: () => callAction("shippin.tenancy.customer.effective_config.get", { customerId }), retry: false });
export const useIntents = (customerId: string) =>
  useQuery({ queryKey: ["catalog", "intents", customerId], queryFn: () => callAction("shippin.tenancy.customer.provision_intent.list", { customerId }), retry: false });

type Send = { method: "POST" | "PUT" | "DELETE"; path: string; body?: unknown };

/** One mutation for every catalog write; refreshes all catalog queries and toasts the result. */
export function useCatalogWrite(done: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (s: Send) => apiSend<unknown>(s.method, s.path, s.body),
    onSuccess: () => toast.success(done),
    onError: (e) => toast.error("Not saved", e.message),
    onSettled: () => void qc.invalidateQueries({ queryKey: ["catalog"] }),
  });
}

/** The same for a catalog write that is an action: run calls it (with its reason), and the catalog refreshes. */
export function useCatalogAction<R>(done: string, run: () => Promise<R>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: run,
    onSuccess: () => toast.success(done),
    onError: (e) => toast.error("Not saved", e.message),
    onSettled: () => void qc.invalidateQueries({ queryKey: ["catalog"] }),
  });
}

export const catalogPaths = {
  plan: (id?: string) => (id ? `${base}/plans/${id}` : `${base}/plans`),
  service: (id?: string) => (id ? `${base}/services/${id}` : `${base}/services`),
  cell: (plan: string, svc: string) => `${base}/plan-services/${plan}/${svc}`,
};
