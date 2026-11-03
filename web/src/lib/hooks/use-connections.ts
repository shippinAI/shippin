import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { Connections, LoginState } from "../api/connect-types";
import { toast } from "../design/components/toast/toast-queue";

const base = (t: string, w: string) => `/v1/tenants/${t}/workspaces/${w}/connections`;
const key = (t: string, w: string) => ["tenant", t, "workspace", w, "connections"];

export const useConnections = (t: string, w: string) =>
  useQuery({ queryKey: key(t, w), queryFn: () => apiGet<Connections>(base(t, w)), enabled: !!t && !!w, refetchInterval: 30_000 });

/** Polls a browser sign-in until it ends; returns the latest state. */
export const useLoginState = (t: string, w: string, provider: string | null) =>
  useQuery({
    queryKey: [...key(t, w), "login", provider],
    queryFn: () => apiGet<LoginState>(`${base(t, w)}/${provider}/login`),
    enabled: !!provider,
    refetchInterval: (q) => (["connected", "failed", "expired"].includes(q.state.data?.state ?? "") ? false : 2000),
  });

export function useStartLogin(t: string, w: string) {
  return useMutation({
    mutationFn: (provider: string) => apiSend<LoginState>("POST", `${base(t, w)}/${provider}/login`),
    onError: (e) => toast.error("Sign-in didn't start", e.message),
  });
}

export function useSubmitCode(t: string, w: string) {
  return useMutation({
    mutationFn: (v: { provider: string; code: string }) => apiSend("POST", `${base(t, w)}/${v.provider}/code`, { code: v.code }),
    onError: (e) => toast.error("Code not accepted", e.message),
  });
}

export function useSaveToken(t: string, w: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { provider: string; token: string }) => apiSend("PUT", `${base(t, w)}/${v.provider}/token`, { token: v.token }),
    onSuccess: () => {
      toast.success("Saved", "Stored in Vaultix and added to the workspace. It comes back on every start.");
      void qc.invalidateQueries({ queryKey: key(t, w) });
    },
    onError: (e) => toast.error("Token not saved", e.message),
  });
}

export function useDisconnectProvider(t: string, w: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (provider: string) => apiSend("DELETE", `${base(t, w)}/${provider}`),
    onSuccess: () => {
      toast.success("Disconnected", "Signed out in the workspace and removed from Vaultix.");
      void qc.invalidateQueries({ queryKey: key(t, w) });
    },
    onError: (e) => toast.error("Not disconnected", e.message),
  });
}

export const refreshConnections = (qc: ReturnType<typeof useQueryClient>, t: string, w: string) =>
  void qc.invalidateQueries({ queryKey: key(t, w) });
