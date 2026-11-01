import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiSend } from "../api/client";
import type { Template, TemplateList } from "../api/types";
import { toast } from "../design/components/toast/toast-queue";

/** Enabled templates + size presets for the create flow. */
export const useTemplates = () =>
  useQuery({ queryKey: ["templates"], queryFn: () => apiGet<TemplateList>("/v1/templates"), staleTime: 60_000 });

/** Provider view: all templates (incl. disabled and startup scripts). */
export const useAdminTemplates = () =>
  useQuery({ queryKey: ["admin", "templates"], queryFn: () => apiGet<TemplateList>("/v1/operator/templates"), retry: false });

export function useSaveTemplate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (t: Partial<Template>) =>
      t.id ? apiSend<Template>("PUT", `/v1/operator/templates/${t.id}`, t) : apiSend<Template>("POST", "/v1/operator/templates", t),
    onSuccess: (t, input) => toast.success(input.id ? `${t.name} updated` : `${t.name} created`),
    onError: (e) => toast.error("Template not saved", e.message),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["admin", "templates"] });
      void qc.invalidateQueries({ queryKey: ["templates"] });
    },
  });
}

export function useDeleteTemplate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (t: Template) => apiSend("DELETE", `/v1/operator/templates/${t.id}`),
    onSuccess: (_d, t) => toast.success(`${t.name} deleted`, "Existing workspaces keep running."),
    onError: (e) => toast.error("Template not deleted", e.message),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["admin", "templates"] });
      void qc.invalidateQueries({ queryKey: ["templates"] });
    },
  });
}
