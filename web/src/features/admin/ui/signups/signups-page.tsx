import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, MailCheck, UserPlus } from "lucide-react";
import { useState } from "react";
import { apiGet, apiSend } from "../../../../lib/api/client";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Select } from "../../../../lib/design/components/select/select";
import { toast } from "../../../../lib/design/components/toast/toast-queue";
import { CatalogPerm, useCan } from "../../../../lib/hooks/use-can";
import { usePlatformSettings, useSetPlatformSetting } from "../../../../lib/hooks/use-platform-settings";
import { PageHeader } from "../../../../template/page-header";

type WaitRow = { id: string; email: string; name: string; company: string; notes: string; status: "pending" | "confirmed" | "invited"; created_at: string };
type Waitlist = { signup: string; mail: boolean; items: WaitRow[] };
type Invited = { email: string; link: string; mailed: boolean };

const status: Record<WaitRow["status"], { label: string; tone: "neutral" | "info" | "success" }> = {
  pending: { label: "Address not confirmed", tone: "neutral" },
  confirmed: { label: "Waiting for an invitation", tone: "info" },
  invited: { label: "Invited", tone: "success" },
};

/** Customers → Sign-ups: who may sign up (LAUNCH-01), the waitlist, and invitations off it. */
export function SignupsPage() {
  const can = useCan();
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["waitlist"], queryFn: () => apiGet<Waitlist>("/v1/operator/waitlist") });
  const mode = usePlatformSettings().data?.items.find((s) => s.key === "directory.signup");
  const setMode = useSetPlatformSetting();
  const [next, setNext] = useState<string | null>(null);
  const [reason, setReason] = useState("");
  const [inviteReason, setInviteReason] = useState("");
  const [invited, setInvited] = useState<Invited | null>(null);
  const invite = useMutation({
    mutationFn: (id: string) => apiSend<Invited>("POST", `/v1/operator/waitlist/${encodeURIComponent(id)}/invite`, { reason: inviteReason }),
    onSuccess: (r) => {
      setInvited(r);
      toast.success(r.mailed ? "Invitation emailed" : "Invitation created", r.mailed ? r.email : "Send the link yourself; it works once.");
      return qc.invalidateQueries({ queryKey: ["waitlist"] });
    },
    onError: (e) => toast.error("Not invited", e.message),
  });
  const writeMode = can("platform.settings.write");
  const writeInvites = can(CatalogPerm.customersWrite);
  const current = next ?? String(mode?.value ?? list.data?.signup ?? "");
  const rows = list.data?.items ?? [];
  const reasonOK = inviteReason.trim().length >= 5;

  const cols: ColumnDef<WaitRow>[] = [
    { id: "who", label: "Person", isRowHeader: true, render: (r) => <div><div className="text-fg">{r.name || r.email}</div>{r.name && <div className="text-sm text-fg-subtle">{r.email}</div>}</div> },
    { id: "company", label: "Company", render: (r) => r.company || "—" },
    { id: "notes", label: "Wants to build", render: (r) => <span className="line-clamp-2">{r.notes || "—"}</span> },
    { id: "status", label: "Status", render: (r) => <Badge tone={status[r.status].tone}>{status[r.status].label}</Badge> },
    { id: "when", label: "Asked", width: "8rem", render: (r) => new Date(r.created_at).toLocaleDateString() },
    { id: "act", label: "", width: "8rem", render: (r) => writeInvites && r.status === "confirmed" && (
      <Button size="sm" variant="primary" isDisabled={!reasonOK || invite.isPending} tooltip={reasonOK ? `Invite ${r.email}` : "Give a reason above first"} onPress={() => invite.mutate(r.id)}>
        <UserPlus className="size-4" /> Invite
      </Button>) },
  ];

  return (
    <>
      <PageHeader eyebrow="Customers" title="Sign-ups" description="Who can create an account, and the people waiting for an invitation. Every change needs a reason and is audited." />
      <Card className="mb-6">
        <CardHeader title="Who can sign up" description={mode?.help} />
        <div className="grid gap-4 p-5 md:grid-cols-[1fr_1fr_auto] md:items-end">
          {mode?.choices ? (
            <Select label="Sign-up" value={current} onChange={setNext} isDisabled={!writeMode} options={mode.choices.map((c) => ({ id: c.value, label: c.label }))} />
          ) : <p className="text-base text-fg-muted">Current: {list.data?.signup ?? "…"}</p>}
          {writeMode && <TextField label="Reason (audited)" value={reason} onChange={setReason} placeholder="Why this changes" />}
          {writeMode && (
            <Button variant="primary" isDisabled={!mode || current === mode.value || reason.trim().length < 5 || setMode.isPending}
              tooltip="Save who can sign up"
              onPress={() => setMode.mutate({ key: "directory.signup", value: current, reason }, { onSuccess: () => { setReason(""); setNext(null); void qc.invalidateQueries({ queryKey: ["waitlist"] }); } })}>Save</Button>
          )}
        </div>
        {!writeMode && <p className="px-5 pb-5 text-sm text-fg-subtle">Changing this needs platform.settings.write.</p>}
      </Card>
      <Card>
        <CardHeader title="Waitlist" description={list.data?.signup === "waitlist"
          ? "The sign-in page links to the waitlist. People confirm their address, then you invite them."
          : "The waitlist is closed, so no one new can join it. Anyone already on it can still be invited."} />
        {list.data?.signup === "waitlist" && !list.data.mail && (
          <p role="alert" className="border-b border-line px-5 pb-4 text-sm text-warning">Mail isn't set up, so no one can join the waitlist: they couldn't confirm their address. The sign-in page doesn't offer it until mail works.</p>
        )}
        {writeInvites && (
          <div className="grid gap-3 border-b border-line px-5 pb-4 md:grid-cols-[1fr_auto] md:items-end">
            <TextField label="Reason for invitations (audited)" value={inviteReason} onChange={setInviteReason} placeholder="e.g. friends and family round" />
            {list.data && !list.data.mail && <p className="text-sm text-fg-subtle">Mail isn't set up, so you'll copy each link and send it yourself.</p>}
          </div>
        )}
        {invited && (
          <div role="status" className="flex flex-wrap items-center gap-3 border-b border-line bg-raised/40 px-5 py-3 text-sm">
            <MailCheck className="size-4 text-success" />
            <span className="text-fg">{invited.mailed ? `Emailed ${invited.email}.` : `Send ${invited.email} this link. It works once and lasts 14 days.`}</span>
            <code className="max-w-full truncate font-mono text-xs text-fg-muted">{invited.link}</code>
            <Button size="sm" tooltip="Copy the invitation link" onPress={() => void navigator.clipboard.writeText(invited.link).then(() => toast.success("Link copied"))}>
              <Copy className="size-4" /> Copy
            </Button>
          </div>
        )}
        {list.isError ? <p role="alert" className="p-5 text-base text-danger">{list.error.message}</p> : (
          <DataTable label="Waitlist" columns={cols} rows={rows} dependencies={[reasonOK, invite.isPending, writeInvites]}
            empty={<EmptyState title="No one is waiting" description={list.data?.signup === "waitlist" ? "People who join from the sign-in page appear here." : "Switch sign-up to “with a public waitlist” to let people ask to join."} />} />
        )}
      </Card>
    </>
  );
}
