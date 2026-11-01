import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { apiGet, apiSend } from "../api/client";
import type { Activation, MySubscriptions } from "../api/subscription-types";
import { toast } from "../design/components/toast/toast-queue";

const key = ["me", "subscriptions"];

export const useSubscriptions = () =>
  useQuery({
    queryKey: key,
    queryFn: () => apiGet<MySubscriptions>("/v1/me/subscriptions"),
    refetchInterval: (q) => (q.state.data?.homes.some((h) => h.utility_state === "starting") ? 3000 : false),
  });

const base = (tenantId: string, provider: string) => `/v1/tenants/${tenantId}/subscriptions/${provider}`;

export function useActivate(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (provider: string) => apiSend<Activation>("POST", `${base(tenantId, provider)}/activate`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: key }),
    onError: (e) => toast.error("Your AI terminal didn't start", e.message),
  });
}

export function useCheckSubscription(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (provider: string) => apiSend("POST", `${base(tenantId, provider)}/check`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: key }),
    onError: (e) => {
      toast.error("Status not checked", e.message);
      void qc.invalidateQueries({ queryKey: key });
    },
  });
}

export function useDisconnect(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (provider: string) => apiSend("DELETE", base(tenantId, provider)),
    onSuccess: () => {
      toast.success("Signed out", "The vendor's own sign-out ran in your AI terminal.");
      void qc.invalidateQueries({ queryKey: key });
    },
    onError: (e) => toast.error("Not signed out", e.message),
  });
}

export function useCloseTerminal(tenantId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiSend("DELETE", `/v1/tenants/${tenantId}/ai-terminal`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: key }),
  });
}

/** While the terminal dialog is open, report it in use every touch_seconds (idle stop otherwise). */
export function useTouchTerminal(tenantId: string, active: boolean, seconds: number) {
  useEffect(() => {
    if (!active || seconds <= 0) return;
    const id = window.setInterval(() => void apiSend("POST", `/v1/tenants/${tenantId}/ai-terminal/touch`).catch(() => {}), seconds * 1000);
    return () => window.clearInterval(id);
  }, [tenantId, active, seconds]);
}
