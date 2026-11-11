import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { CatalogService, CoverageTier, Customer, EffectiveConfig, Intent, Items, Plan, PlanService } from "../api/catalog-types";
import { toast } from "../design/components/toast/toast-queue";

const base = "/v1/operator/catalog";
const get = <T,>(key: unknown[], path: string) => useQuery({ queryKey: ["catalog", ...key], queryFn: () => apiGet<T>(path), retry: false });

export const usePlans = () => get<Items<Plan>>(["plans"], `${base}/plans`);
export const useCoverageTiers = () => get<Items<CoverageTier>>(["coverage"], `${base}/coverage`);
export const useServices = () => get<Items<CatalogService>>(["services"], `${base}/services`);
export const usePlanServices = () => get<Items<PlanService>>(["plan-services"], `${base}/plan-services`);
export const useCustomers = () => get<Items<Customer>>(["customers"], "/v1/operator/customers");
export const useCustomer = (id: string) => get<Customer>(["customers", id], `/v1/operator/customers/${id}`);
export const useEffective = (id: string) => get<EffectiveConfig>(["effective", id], `/v1/operator/customers/${id}/effective-config`);
export const useIntents = (id: string) => get<Items<Intent>>(["intents", id], `/v1/operator/customers/${id}/provision-intents`);

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

export const catalogPaths = {
  plan: (id?: string) => (id ? `${base}/plans/${id}` : `${base}/plans`),
  coverage: (id?: string) => (id ? `${base}/coverage/${id}` : `${base}/coverage`),
  service: (id?: string) => (id ? `${base}/services/${id}` : `${base}/services`),
  cell: (plan: string, svc: string) => `${base}/plan-services/${plan}/${svc}`,
  customer: (id?: string) => (id ? `/v1/operator/customers/${id}` : "/v1/operator/customers"),
  tenantService: (id: string, svc: string) => `/v1/operator/customers/${id}/services/${svc}`,
  intent: (id: string) => `/v1/operator/customers/${id}/provision-intent`,
};
