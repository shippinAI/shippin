import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { timeAgo } from "../../../lib/format";
import { useMe } from "../../../lib/hooks/use-api";
import { addPasskey, deletePasskey, listPasskeys, passkeyProblem } from "../../../lib/passkey";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { PageHeader } from "../../../template/page-header";

/** A person's own way of signing in. A passkey is also the second factor a staff role needs. */
export function SecurityPage() {
  const t = useText();
  const me = useMe().data;
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["passkeys"], queryFn: listPasskeys });
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["passkeys"] });
    void qc.invalidateQueries({ queryKey: ["me"] });
  };
  const add = useMutation({
    mutationFn: () => addPasskey({ id: me?.user_id ?? "", email: me?.email ?? "", name: me?.display_name ?? "" }),
    onSuccess: () => { toast.success(t("security.added")); refresh(); },
  });
  const remove = useMutation({ mutationFn: deletePasskey, onSuccess: refresh, onError: (e) => toast.error(t("security.removeFailed"), e.message) });
  const items = q.data?.items ?? [];
  return (
    <>
      <PageHeader eyebrow={t("security.eyebrow")} title={t("security.title")} description={t("security.description")} />
      <Card className="overflow-hidden" data-testid="passkeys">
        <CardHeader title={t("security.passkeys")} description={t("security.passkeysHelp")}
          actions={<Button size="sm" variant="primary" isDisabled={add.isPending || !me} tooltip={t("security.addTooltip")} onPress={() => add.mutate()}><KeyRound className="size-4" /> {t("security.add")}</Button>} />
        {add.isError && <p role="alert" data-testid="passkey-error" className="px-5 pt-4 text-sm text-danger">{passkeyProblem(add.error)}</p>}
        {items.length === 0 ? <p className="p-5 text-sm text-fg-muted">{t("security.none")}</p> : (
          <ul className="divide-y divide-line">
            {items.map((k) => (
              <li key={k.id} className="flex items-center justify-between gap-3 px-5 py-3 text-sm" data-testid="passkey">
                <span className="text-fg">{t("security.addedAgo", { when: timeAgo(k.created_at) })}</span>
                <Button size="sm" variant="ghost" tooltip={t("security.removeTooltip")} isDisabled={remove.isPending} onPress={() => remove.mutate(k.id)}>{t("security.remove")}</Button>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </>
  );
}
