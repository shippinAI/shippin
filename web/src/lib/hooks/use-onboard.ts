import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiSend } from "../api/client";
import { toast } from "../design/components/toast/toast-queue";

type Created = { tenant_id: string; name: string; status: string };

export function useOnboard() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => apiSend<Created>("POST", "/v1/onboarding", { name }),
    onSuccess: async (t) => {
      toast.success(`${t.name} is ready`, "Create your first workspace.");
      await qc.invalidateQueries({ queryKey: ["me"] });
    },
    onError: (e) => toast.error("Setup didn't finish", e.message),
  });
}
