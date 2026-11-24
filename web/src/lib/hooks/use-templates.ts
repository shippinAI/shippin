import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { callAction } from "../api/actions";
import type { Template, TemplateFields } from "../api/actions.gen";
import { toast } from "../design/components/toast/toast-queue";

/** What a development space can start from: the templates turned on, the sizes and what the server accepts. */
export const useDevspaceOptions = () =>
  useQuery({ queryKey: ["devspace-options"], queryFn: () => callAction("shippin.runtime.devspace_option.get"), staleTime: 60_000 });

/** Staff: every template, the ones turned off too, with their start commands. */
export const useAdminTemplates = () =>
  useQuery({ queryKey: ["admin", "templates"], queryFn: () => callAction("shippin.runtime.template.list"), retry: false });

function useRefresh() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: ["admin", "templates"] });
    void qc.invalidateQueries({ queryKey: ["devspace-options"] });
  };
}

export function useSaveTemplate() {
  const onSettled = useRefresh();
  return useMutation({
    mutationFn: ({ id, fields, reason }: { id?: string; fields: TemplateFields; reason: string }) =>
      id
        ? callAction("shippin.runtime.template.update", { templateId: id }, { ...fields, reason })
        : callAction("shippin.runtime.template.create", {}, { ...fields, reason }),
    onSuccess: (t, input) => toast.success(input.id ? `${t.name} updated` : `${t.name} created`),
    onError: (e) => toast.error("Template not saved", e.message),
    onSettled,
  });
}

export function useDeleteTemplate() {
  const onSettled = useRefresh();
  return useMutation({
    mutationFn: ({ template, reason }: { template: Template; reason: string }) =>
      callAction("shippin.runtime.template.delete", { templateId: template.id }, { reason }),
    onSuccess: (_d, { template }) => toast.success(`${template.name} deleted`, "Development spaces made from it keep running."),
    onError: (e) => toast.error("Template not deleted", e.message),
    onSettled,
  });
}
