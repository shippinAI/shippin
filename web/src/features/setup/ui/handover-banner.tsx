import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Circle, KeyRound, UserRoundPlus } from "lucide-react";
import { useState, type ReactNode } from "react";
import { apiGet, apiSend } from "../../../lib/api/client";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { RecoveryCodes } from "../../auth/ui/recovery-codes";

type Handover = { pending: boolean; you_are_setup?: boolean; username?: string; password_set?: boolean; owners?: string[] };
const key = ["setup-handover"];

/** REL-03: while the installation still runs on the built-in setup account, the owner sees two steps,
 * in order, on every page: set the setup account's password, then create their own owner account. */
export function HandoverBanner() {
  const q = useQuery({ queryKey: key, queryFn: () => apiGet<Handover>("/v1/setup/handover"), retry: false, staleTime: 30_000 });
  const [open, setOpen] = useState<"password" | "owner" | null>(null);
  const h = q.data;
  // The dialogs stay mounted while open: finishing step 2 ends the takeover, and the new account's
  // recovery codes must still be shown.
  const dialogs = <>
    <PasswordDialog isOpen={open === "password"} onClose={() => setOpen(null)} />
    <OwnerDialog isOpen={open === "owner"} onClose={() => setOpen(null)} />
  </>;
  if (!h?.pending) return <>{null}{dialogs}</>;
  const passwordDone = h.password_set === true;
  const ownerDone = (h.owners?.length ?? 0) > 0;
  return <>
    <section role="region" aria-label="Take over this installation" className="mx-4 mt-4 rounded-xl border border-warning/40 bg-warning/8 p-5 md:mx-8">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="max-w-2xl">
          <h2 className="text-lg font-semibold text-fg">Take over this installation</h2>
          <p className="mt-1 text-sm text-fg-muted">
            {h.you_are_setup
              ? <>You're signed in as the built-in <span className="font-mono text-fg">{h.username}</span> account, which was used to set this installation up. Make it yours in two steps.</>
              : <>This installation was set up with the built-in <span className="font-mono text-fg">{h.username}</span> account. Finish taking it over.</>}
          </p>
        </div>
      </div>
      <ol className="mt-4 grid gap-3 md:grid-cols-2">
        <Step n={1} done={passwordDone} title="Set a new password for the setup account"
          body={passwordDone ? "Done. The password used during setup no longer works." : "Whoever ran setup knows its current password. Setting your own signs everyone else out of it."}
          action={!passwordDone && <Button variant="primary" size="sm" tooltip="Choose a new password for the setup account" onPress={() => setOpen("password")}><KeyRound className="size-4" /> Set password</Button>} />
        <Step n={2} done={ownerDone} title="Create your own owner account"
          body={ownerDone ? `Done: ${h.owners?.join(", ")}. Sign in with it from now on.` : "Your everyday sign-in, with its own recovery codes. Keep the setup account for emergencies."}
          action={!ownerDone && <Button variant={passwordDone ? "primary" : "secondary"} size="sm" tooltip="Create your own owner account" onPress={() => setOpen("owner")}><UserRoundPlus className="size-4" /> Create account</Button>} />
      </ol>
    </section>
    {dialogs}
  </>;
}

function Step({ n, done, title, body, action }: { n: number; done: boolean; title: string; body: string; action: ReactNode }) {
  return (
    <li className="flex gap-3 rounded-lg border border-line bg-surface p-4">
      {done ? <CheckCircle2 aria-hidden className="mt-0.5 size-5 shrink-0 text-success" /> : <Circle aria-hidden className="mt-0.5 size-5 shrink-0 text-fg-subtle" />}
      <div className="min-w-0 flex-1">
        <p className="font-medium text-fg"><span className="sr-only">Step {n}{done ? ", done" : ""}: </span>{title}</p>
        <p className="mt-1 text-sm text-fg-muted">{body}</p>
        {action && <div className="mt-3">{action}</div>}
      </div>
    </li>
  );
}

function PasswordDialog({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [saved, setSaved] = useState(false);
  const save = useMutation({
    mutationFn: () => apiSend<{ sessions_ended: number }>("POST", "/v1/setup/handover/password", { password }),
    onSuccess: (r) => {
      toast.success("Setup account password set", r.sessions_ended ? `${r.sessions_ended} other sign-in(s) of it were signed out.` : "No one else was signed in to it.");
      setPassword(""); setConfirm(""); setSaved(false); onClose();
      return qc.invalidateQueries({ queryKey: key });
    },
    onError: (e) => toast.error("Not saved", e.message),
  });
  const mismatch = confirm !== "" && confirm !== password;
  const ok = password.length >= 12 && password === confirm && saved;
  return (
    <ModalDialog isOpen={isOpen} onOpenChange={(o) => !o && onClose()} title="Set the setup account's password"
      description="Choose a password only you know, and save it in your password manager. The password used during setup stops working, and every other sign-in of this account is signed out.">
      <form className="grid gap-4 px-6 py-5" onSubmit={(e) => { e.preventDefault(); if (ok) save.mutate(); }}>
        <TextField label="New password" type="password" autoComplete="new-password" value={password} onChange={setPassword} description="At least 12 characters." />
        <TextField label="Confirm new password" type="password" autoComplete="new-password" value={confirm} onChange={setConfirm} />
        {mismatch && <p role="alert" className="text-sm text-danger">The passwords don't match.</p>}
        <label className="flex items-start gap-2 text-sm text-fg">
          <input type="checkbox" className="mt-1" checked={saved} onChange={(e) => setSaved(e.target.checked)} />
          I've saved this password somewhere safe.
        </label>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" tooltip="Close without saving" onPress={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" isDisabled={!ok || save.isPending} tooltip={ok ? "Save the new password" : "Enter it twice and confirm you saved it"}>{save.isPending ? "Saving…" : "Save password"}</Button>
        </div>
      </form>
    </ModalDialog>
  );
}

function OwnerDialog({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const [f, setF] = useState({ username: "", name: "", email: "", password: "", confirm: "" });
  const [made, setMade] = useState<{ username: string; recovery_codes: string[] } | null>(null);
  const create = useMutation({
    mutationFn: () => apiSend<{ username: string; recovery_codes: string[] }>("POST", "/v1/setup/handover/owner", { username: f.username, name: f.name, email: f.email, password: f.password }),
    onSuccess: (r) => { setMade(r); return qc.invalidateQueries({ queryKey: key }); },
    onError: (e) => toast.error("Not created", e.message),
  });
  const set = (k: keyof typeof f) => (v: string) => setF({ ...f, [k]: v });
  const ok = f.username.length >= 3 && f.name.trim() !== "" && f.email.includes("@") && f.password.length >= 12 && f.password === f.confirm;
  const close = () => { setMade(null); setF({ username: "", name: "", email: "", password: "", confirm: "" }); onClose(); };
  return (
    <ModalDialog isOpen={isOpen} onOpenChange={(o) => !o && close()} title={made ? "Your account is ready" : "Create your own owner account"}
      description={made ? undefined : "This is the account you'll use every day. It owns the installation, like the setup account."}>
      {made ? (
        <div className="grid gap-4 px-6 py-5">
          <RecoveryCodes codes={made.recovery_codes} next={<>
            <Button variant="primary" tooltip={`Sign out, then sign in as ${made.username}`} onPress={() => window.location.assign("/auth/logout")}>I saved them, sign in as {made.username}</Button>
            <Button variant="ghost" tooltip="Stay signed in as the setup account for now" onPress={close}>Later</Button>
          </>} />
        </div>
      ) : (
        <form className="grid gap-4 px-6 py-5" onSubmit={(e) => { e.preventDefault(); if (ok) create.mutate(); }}>
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField label="Username" autoComplete="username" value={f.username} onChange={set("username")} description="3 to 32 letters, digits, dots, dashes or underscores." />
            <TextField label="Your name" autoComplete="name" value={f.name} onChange={set("name")} />
          </div>
          <TextField label="Email address" type="email" autoComplete="email" value={f.email} onChange={set("email")} />
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField label="Password" type="password" autoComplete="new-password" value={f.password} onChange={set("password")} description="At least 12 characters." />
            <TextField label="Confirm password" type="password" autoComplete="new-password" value={f.confirm} onChange={set("confirm")} />
          </div>
          {f.confirm !== "" && f.confirm !== f.password && <p role="alert" className="text-sm text-danger">The passwords don't match.</p>}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" tooltip="Close without creating" onPress={close}>Cancel</Button>
            <Button type="submit" variant="primary" isDisabled={!ok || create.isPending} tooltip={ok ? "Create the account" : "Fill in every field"}>{create.isPending ? "Creating…" : "Create account"}</Button>
          </div>
        </form>
      )}
    </ModalDialog>
  );
}
