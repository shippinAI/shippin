import { useState } from "react";
import type { BypassCert, BypassInfo } from "../../../lib/api/gold-sign-types";
import { Button } from "../../../lib/design/components/button/button";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { timeLeft } from "../../../lib/gold-sign/bypass-model";
import { signDecision } from "../../../lib/gold-sign/webauthn";
import { useRelay } from "../use-gold-sign";

const when = (s: number) => new Date(s * 1000).toLocaleString();

function CertCard({ c, info }: { c: BypassCert; info: BypassInfo }) {
  const relay = useRelay();
  const [busy, setBusy] = useState(false);
  const revoke = async () => {
    setBusy(true);
    try {
      const env = await signDecision({ type: "revoke", cid: c.cid, decision: "revoke", ts: Math.floor(Date.now() / 1000) }, info.rp_id, info.keys);
      await relay.mutateAsync(env);
      toast.success("Revoke sent", "Gold verifies your passkey and adds it to the revocation list.");
    } catch (e) {
      toast.error("Revoke not sent", e instanceof Error ? e.message : "Passkey cancelled");
    } finally {
      setBusy(false);
    }
  };
  return (
    <li className="rounded-lg border border-danger/30 bg-danger/5 p-3">
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-[12.5px]">
        <dt className="text-fg-subtle">Agent</dt><dd className="font-medium text-fg">{c.holder_agent}</dd>
        <dt className="text-fg-subtle">Scope</dt><dd className="font-mono break-all text-fg">{c.scope.join(", ")}</dd>
        <dt className="text-fg-subtle">Time left</dt><dd className="text-fg">{timeLeft(info.status.now, c.exp)} (until {when(c.exp)})</dd>
        <dt className="text-fg-subtle">Valid from</dt><dd>{when(c.nbf)}</dd>
        <dt className="text-fg-subtle">Uses</dt><dd>{c.uses} of {c.max_uses || "∞"}</dd>
        <dt className="text-fg-subtle">Cert id</dt><dd className="font-mono break-all">{c.cid}</dd>
        {c.reason && <><dt className="text-fg-subtle">Reason</dt><dd>{c.reason}</dd></>}
      </dl>
      {c.revoke_sent ? <p className="mt-2 text-xs text-warning">Revoke sent; still active until Gold confirms it.</p>
        : info.can_revoke && (
          <Button variant="danger" size="md" className="mt-3 w-full justify-center" isDisabled={busy || info.keys.length === 0}
            tooltip="Revoke with your passkey" onPress={revoke}>{busy ? "Waiting for passkey…" : "Revoke"}</Button>
        )}
    </li>
  );
}

/** Popover body: full details of each active certificate and a passkey-signed Revoke. */
export function BypassDetails({ info }: { info: BypassInfo }) {
  const active = info.status.active;
  const ago = info.status.sync_at ? `${Math.max(0, info.status.now - info.status.sync_at)}s ago` : "never";
  if (!info.status.known && active.length === 0)
    return <p data-testid="bypass-unknown">Unknown: Gold last synced {ago}. A bypass may be active; check Gold before relying on this.</p>;
  if (active.length === 0) return <p data-testid="bypass-none">No bypass active. Every Gold change needs a per-ticket approval.</p>;
  return (
    <ul className="max-h-[60vh] space-y-2 overflow-y-auto" aria-label="Active bypass certificates">
      {active.map((c) => <CertCard key={c.cid} c={c} info={info} />)}
    </ul>
  );
}
