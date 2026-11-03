import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../../lib/api/client";
import type { CloudImport, KeyringLogin, VaultixAccount, VaultixSecret } from "./vaultix-types";

const q = (env: string) => `?env=${encodeURIComponent(env)}`;
const k = (key: string) => encodeURIComponent(key);

export const useVaultixAccount = () =>
  useQuery({ queryKey: ["vaultix", "account"], queryFn: () => apiGet<VaultixAccount>("/v1/vaultix/account"), retry: false });

export const useVaultixSecrets = (env: string) =>
  useQuery({ queryKey: ["vaultix", "secrets", env], enabled: !!env, retry: false,
    queryFn: () => apiGet<{ secrets: VaultixSecret[] }>(`/v1/vaultix/secrets${q(env)}`) });

function useInvalidate() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: ["vaultix"] });
}

export function useSetSecret(env: string) {
  const done = useInvalidate();
  return useMutation({ onSuccess: done,
    mutationFn: (s: { key: string; value: string }) => apiSend<VaultixSecret>("PUT", `/v1/vaultix/secrets/${k(s.key)}${q(env)}`, { value: s.value }) });
}

export function useDeleteSecret(env: string) {
  const done = useInvalidate();
  return useMutation({ onSuccess: done, mutationFn: (key: string) => apiSend("DELETE", `/v1/vaultix/secrets/${k(key)}${q(env)}`) });
}

export const revealSecret = (env: string, key: string) =>
  apiSend<{ value: string }>("POST", `/v1/vaultix/secrets/${k(key)}/reveal${q(env)}`).then((r) => r.value);

export function useKeyringLogin() {
  const done = useInvalidate();
  return useMutation({ onSuccess: done, mutationFn: () => apiSend<KeyringLogin>("POST", "/v1/vaultix/keyring") });
}

export function useCloudImport() {
  const done = useInvalidate();
  return useMutation({ onSuccess: done, mutationFn: (b: CloudImport) => apiSend<{ imported: number }>("POST", "/v1/vaultix/import", b) });
}
