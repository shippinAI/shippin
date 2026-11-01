import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiGet, apiSend } from "../api/client";
import { reauthenticate } from "./use-admin-access";

// Staff mode is a server-side session (fresh IdP sign-in + reason, fixed expiry). The server
// writes the session and its audit entry together; this only reads and drives it.
export type StaffSession = { id: string; reason: string; from_tenant?: string; started_at: string; expires_at: string };
export type StaffModeState = { active: boolean; required: boolean; minutes: number; session?: StaffSession };

const KEY = ["admin", "staff-mode"];

export function useStaffMode() {
  return useQuery({ queryKey: KEY, queryFn: () => apiGet<StaffModeState>("/v1/operator/staff-mode"), refetchInterval: 30_000, retry: false });
}

export function useEnterStaffMode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (b: { reason: string; from_tenant: string }) => apiSend<StaffModeState>("POST", "/v1/operator/staff-mode", b),
    onSuccess: (s) => qc.setQueryData(KEY, s),
    onError: (e) => {
      // 428: the IdP sign-in is too old; sign in again (prompt=login, max_age=0) and come back.
      if (e instanceof ApiError && e.status === 428) window.setTimeout(reauthenticate, 800);
    },
  });
}

export function useExitStaffMode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiSend<{ console_url: string }>("POST", "/v1/operator/staff-mode/exit", {}),
    onSuccess: (r) => {
      qc.removeQueries({ queryKey: KEY });
      window.location.assign(r.console_url);
    },
  });
}
