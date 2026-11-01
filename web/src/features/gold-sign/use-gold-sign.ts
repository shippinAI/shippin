import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../../lib/api/client";
import type { BypassInfo, GoldFacts, GoldKey, GoldTickets, RegBegin } from "../../lib/api/gold-sign-types";
import type { Envelope } from "../../lib/gold-sign/webauthn";

export const useGoldTickets = () =>
  useQuery({ queryKey: ["gold-sign", "tickets"], queryFn: () => apiGet<GoldTickets>("/v1/gold-sign/tickets"), refetchInterval: 20_000, retry: false });

export const useGoldTicket = (id: string) =>
  useQuery({ queryKey: ["gold-sign", "ticket", id], queryFn: () => apiGet<GoldFacts>(`/v1/gold-sign/tickets/${encodeURIComponent(id)}`), retry: false });

/** Polled on every page: the header badge must never be stale for long. */
export const useBypass = () =>
  useQuery({ queryKey: ["gold-sign", "bypass"], queryFn: () => apiGet<BypassInfo>("/v1/gold-sign/bypass"), refetchInterval: 15_000, retry: false });

export const useGoldKeys = () => useQuery({ queryKey: ["gold-sign", "keys"], queryFn: () => apiGet<GoldKey[]>("/v1/gold-sign/keys"), retry: false });

/** Relays a signed envelope. The console only forwards it; Gold verifies and writes the approval. */
export function useRelay() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (e: Envelope) => apiSend<{ status: string }>("POST", "/v1/gold-sign/envelopes", e),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["gold-sign"] }),
  });
}

export const beginRegistration = () => apiSend<RegBegin>("POST", "/v1/gold-sign/keys/begin");
export const finishRegistration = (body: unknown) => apiSend<GoldKey>("POST", "/v1/gold-sign/keys", body);
