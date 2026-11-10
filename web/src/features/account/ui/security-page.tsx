import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, LockKeyhole } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { timeAgo } from "../../../lib/format";
import { useMe } from "../../../lib/hooks/use-api";
import { addPasskey, deletePasskey, listPasskeys, passkeyProblem } from "../../../lib/passkey";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { PageHeader } from "../../../template/page-header";
import { apiSend } from "../../../lib/api/client";

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
        {add.isError && <p role="alert" data-testid="passkey-error" className="px-5 pt-4 text-base text-danger">{passkeyProblem(add.error)}</p>}
        {items.length === 0 ? <p className="p-5 text-base text-fg-muted">{t("security.none")}</p> : (
          <ul className="divide-y divide-line">
            {items.map((k) => (
              <li key={k.id} className="flex items-center justify-between gap-3 px-5 py-3 text-base" data-testid="passkey">
                <span className="text-fg">{t("security.addedAgo", { when: timeAgo(k.created_at) })}</span>
                <Button size="sm" variant="ghost" tooltip={t("security.removeTooltip")} isDisabled={remove.isPending} onPress={() => remove.mutate(k.id)}>{t("security.remove")}</Button>
              </li>
            ))}
          </ul>
        )}
      </Card>
      <Card className="mt-5 overflow-hidden" data-testid="password-management">
        <CardHeader title="Password" description="Use a unique password of at least 12 characters. For accounts created before password sign-in was available, leave current password empty and use this signed-in session to set the first one." />
        <form className="grid max-w-xl gap-3 p-5" onSubmit={(e) => {
          e.preventDefault(); const form = e.currentTarget; const f = new FormData(form); const password = String(f.get("password") ?? "");
          if (password !== String(f.get("confirm") ?? "")) { toast.error("Password not changed", "The new passwords do not match."); return; }
          void apiSend("PUT", "/v1/directory/me/password", { current_password: f.get("current"), password }).then(() => { form.reset(); toast.success("Password updated", "You can now sign in with your username or email and password."); }).catch((err: unknown) => toast.error("Password not changed", err instanceof Error ? err.message : "The panel could not update the password."));
        }}>
          <input name="current" type="password" autoComplete="current-password" placeholder="Current password (if already set)" aria-label="Current password" className="rounded-lg border border-line-control bg-canvas px-3 py-2 text-base" />
          <div className="grid gap-3 sm:grid-cols-2"><input name="password" type="password" required minLength={12} maxLength={1024} autoComplete="new-password" placeholder="New password (12+ characters)" aria-label="New password" className="rounded-lg border border-line-control bg-canvas px-3 py-2 text-base" /><input name="confirm" type="password" required minLength={12} autoComplete="new-password" placeholder="Confirm new password" aria-label="Confirm new password" className="rounded-lg border border-line-control bg-canvas px-3 py-2 text-base" /></div>
          <Button type="submit" variant="primary" className="w-fit" tooltip="Set or change the password for this local account"><LockKeyhole className="size-4" /> Set password</Button>
        </form>
      </Card>
    </>
  );
}
