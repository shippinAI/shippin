import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { SettingsView } from "../../../../lib/api/actions.gen";
import { callAction } from "../../../../lib/api/actions";
import { toast } from "../../../../lib/design/components/toast/toast-queue";
import { useAdminAccess } from "../../../../lib/hooks/use-admin";
import { checkReason, problemOf, type SettingPatch, type StoreForm, storeBody } from "./model";

const storeKey = ["platform", "secret-store"];
const settingsKey = ["platform", "settings"];

/** Whether the signed-in staff member may change settings (reading needs platform.settings.read, which the
 * navigation already requires). */
export const useCanWriteSettings = () => (useAdminAccess().data?.staff?.permissions ?? []).includes("platform.settings.write");

/** The installation's secret store: whether Vaultix is connected, where, and its last check. Never its client secret. */
export const useSecretStore = () =>
  useQuery({ queryKey: storeKey, queryFn: () => callAction("shippin.platform.secret_store.get"), retry: false });

/** Connect, or change the identity. The server checks the connection before it saves; a failure says which step. */
export function useSaveSecretStore() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ form, reason }: { form: StoreForm; reason: string }) => callAction("shippin.platform.secret_store.update", {}, { ...storeBody(form), reason }),
    onSuccess: (c) => {
      qc.setQueryData(storeKey, c);
      toast.success("Secret store saved", "The panel signed in to Vaultix and wrote, read back and deleted a check value.");
      void qc.invalidateQueries({ queryKey: settingsKey });
    },
  });
}

/** Check the saved connection again. A failed check is a result shown on the page, not an error. */
export function useTestSecretStore() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => callAction("shippin.platform.secret_store.test", {}, { reason: checkReason }),
    onSuccess: (c) => {
      qc.setQueryData(storeKey, c);
      if (c.check_error) toast.error("The check failed", "The page says which step failed.");
      else toast.success("The secret store works", "Signed in, and wrote, read and deleted a check value.");
    },
    onError: (e) => toast.error("Couldn't check", problemOf(e).message),
  });
}

/** Disconnect. Refused while settings keep keys in the store. */
export function useDisconnectSecretStore() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (reason: string) => callAction("shippin.platform.secret_store.delete", {}, { reason }),
    onSuccess: () => {
      toast.success("Secret store disconnected", "The keys already in Vaultix stay there.");
      void qc.invalidateQueries({ queryKey: storeKey });
      void qc.invalidateQueries({ queryKey: settingsKey });
    },
  });
}

/** Every settings section, for the section list. */
export const useSettingSections = () =>
  useQuery({ queryKey: [...settingsKey, "list"], queryFn: () => callAction("shippin.platform.setting.list"), retry: false });

/** One section, as the form edits it. */
export const useSettingSection = (settingId: string) =>
  useQuery({
    queryKey: [...settingsKey, "one", settingId],
    queryFn: () => callAction("shippin.platform.setting.get", { settingId }),
    enabled: !!settingId,
    retry: false,
    // The form edits this copy: it changes only on a save or a reload, never under someone typing.
    // A save based on a copy someone else has since changed is refused and offers the reload.
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });

/** A section's saved versions. */
export const useSettingHistory = (settingId: string, enabled: boolean) =>
  useQuery({
    queryKey: [...settingsKey, "history", settingId],
    queryFn: () => callAction("shippin.platform.setting.version.list", { settingId }),
    enabled: !!settingId && enabled,
    retry: false,
  });

/** Save a section's changed fields against its version. Problems are shown in the form, not as toasts. */
export function useSaveSetting(settingId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ patch, reason }: { patch: SettingPatch; reason: string }) =>
      callAction("shippin.platform.setting.update", { settingId }, { ...patch, reason }),
    onSuccess: (v: SettingsView) => {
      qc.setQueryData([...settingsKey, "one", settingId], v);
      toast.success(`${v.title} saved`, "It takes effect without a restart.");
      void qc.invalidateQueries({ queryKey: [...settingsKey, "list"] });
      void qc.invalidateQueries({ queryKey: [...settingsKey, "history", settingId] });
      void qc.invalidateQueries({ queryKey: storeKey });
    },
  });
}
