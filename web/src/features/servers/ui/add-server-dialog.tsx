import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { TextField } from "../../../lib/design/components/field/text-field";
import { HelpPopover } from "../../../lib/design/components/popover/help-popover";
import type { EnrollToken } from "../../../lib/api/server-types";
import { useCreateServer, useIssueToken } from "../../../lib/hooks/use-servers";
import { TokenReveal } from "./token-reveal";

const slugRe = /^[a-z0-9][a-z0-9-]{0,39}$/;
type Props = { tenantId: string; isOpen: boolean; onOpenChange: (o: boolean) => void };

/** Add server: name it, then show a single-use enrollment token once. */
export function AddServerDialog({ tenantId, isOpen, onOpenChange }: Props) {
  const create = useCreateServer(tenantId);
  const issue = useIssueToken(tenantId);
  const [label, setLabel] = useState("");
  const [slug, setSlug] = useState("");
  const [site, setSite] = useState("");
  const [token, setToken] = useState<EnrollToken | null>(null);
  const close = () => {
    setToken(null); setLabel(""); setSlug(""); setSite("");
    onOpenChange(false);
  };
  const valid = label.trim().length > 0 && slugRe.test(slug) && slugRe.test(site);
  const submit = () =>
    create.mutate({ label: label.trim(), slug, site }, {
      onSuccess: (s) => issue.mutate(s.id, { onSuccess: setToken }),
    });
  const busy = create.isPending || issue.isPending;
  return (
    <ModalDialog isOpen={isOpen} onOpenChange={(o) => (o ? onOpenChange(true) : close())}
      title={token ? "Enrollment token" : "Add server"}
      description={token ? "Give this to hostd on the new server." : "Register a machine. It stays Waiting to enroll until hostd checks in."}>
      {token ? <TokenReveal token={token} onDone={close} /> : (
        <div className="space-y-4 px-6 py-5">
          <TextField label="Name" value={label} onChange={setLabel} placeholder="Gold (home)" autoFocus />
          <TextField label="Slug" value={slug} onChange={(v) => setSlug(v.toLowerCase())} placeholder="gold" mono
            description="Lowercase letters, digits and dashes. Used in IDs and logs." />
          <div className="flex items-end gap-2">
            <div className="flex-1">
              <TextField label="Site" value={site} onChange={(v) => setSite(v.toLowerCase())} placeholder="gold" mono />
            </div>
            <HelpPopover title="Sites and enrollment">
              <p className="text-[13px] text-fg-muted">A site is where the machine lives (for example <code>gold</code> at home). After you add it you get a
                one-time token. hostd trades it for its own credential and then sends a heartbeat on a fixed interval. A server that misses
                several heartbeats shows as <em>Stale</em>. Planned sites (Texas, Wisconsin, Virginia) are listed but never contacted.</p>
            </HelpPopover>
          </div>
          <div className="flex justify-end gap-2 border-t border-line pt-4">
            <Button variant="ghost" tooltip="Close without adding" onPress={close}>Cancel</Button>
            <Button variant="primary" isDisabled={!valid || busy} onPress={submit}
              tooltip={valid ? "Add the server and show its one-time token" : "Fill in a name, a slug and a site"}>
              {busy ? "Adding…" : "Add and get token"}
            </Button>
          </div>
        </div>
      )}
    </ModalDialog>
  );
}
