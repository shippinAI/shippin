import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { callAction } from "../api/actions";
import { toast } from "../design/components/toast/toast-queue";

/** The plans, each with its services: the plan x service matrix in one read. */
export const usePlans = () =>
  useQuery({ queryKey: ["catalog", "plans"], queryFn: () => callAction("shippin.tenancy.plan.list"), retry: false });
export const useCoverageTiers = () =>
  useQuery({ queryKey: ["catalog", "coverage"], queryFn: () => callAction("shippin.tenancy.coverage_tier.list"), retry: false });
export const useServices = () =>
  useQuery({ queryKey: ["catalog", "services"], queryFn: () => callAction("shippin.services.catalog_service.list"), retry: false });
/** The runner sizes customers' build jobs can ask for (runs-on: shippin-<id>). */
export const useRunnerSizes = () =>
  useQuery({ queryKey: ["catalog", "runner-sizes"], queryFn: () => callAction("shippin.services.runner_size.list"), retry: false });
export const useCustomers = () =>
  useQuery({ queryKey: ["catalog", "customers"], queryFn: () => callAction("shippin.tenancy.customer.list"), retry: false });
export const useCustomer = (customerId: string) =>
  useQuery({
    queryKey: ["catalog", "customers", customerId],
    queryFn: () => callAction("shippin.tenancy.customer.get", { customerId }),
    retry: false,
  });
export const useEffective = (customerId: string) =>
  useQuery({
    queryKey: ["catalog", "effective", customerId],
    queryFn: () => callAction("shippin.tenancy.customer.effective_config.get", { customerId }),
    retry: false,
  });
export const useIntents = (customerId: string) =>
  useQuery({
    queryKey: ["catalog", "intents", customerId],
    queryFn: () => callAction("shippin.tenancy.customer.provision_intent.list", { customerId }),
    retry: false,
  });

/** One mutation for catalog changes made through actions (callAction); refreshes all catalog queries and toasts the result. */
export function useCatalogAction(done: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (run: () => Promise<unknown>) => run(),
    onSuccess: () => toast.success(done),
    onError: (e) => toast.error("Not saved", e.message),
    onSettled: () => void qc.invalidateQueries({ queryKey: ["catalog"] }),
  });
}
