import { Globe, Lock, Plus, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { TextField } from "../../../lib/design/components/field/text-field";
import type { Workspace } from "../../../lib/api/types";
import { useWorkspaceAccess } from "../../../lib/hooks/use-workspaces";

const hostRe = /^(?=.{4,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/;

/** Custom hostnames served over HTTPS with an automatic certificate. */
export function DomainsTab({ ws, canManage }: { ws: Workspace; canManage: boolean }) {
  const save = useWorkspaceAccess(ws.tenant_id, ws.id, "domains");
  const [domains, setDomains] = useState<string[]>(ws.domains ?? []);
  const [host, setHost] = useState("");
  useEffect(() => setDomains(ws.domains ?? []), [ws.domains]);
  const h = host.trim().toLowerCase();
  const valid = hostRe.test(h) && !domains.includes(h);
  const commit = (next: string[]) => { setDomains(next); save.mutate({ domains: next }); };
  const item = (d: string, builtIn: boolean) => (
    <div key={d} className="flex items-center gap-3 border-b border-line px-5 py-3 text-base">
      <Globe className="size-4 text-fg-subtle" />
      <span className="flex-1 font-mono text-sm text-fg">{d}</span>
      <Badge tone="success"><Lock className="size-3" /> HTTPS</Badge>
      {builtIn ? <span className="w-8 text-xs text-fg-subtle">Default</span> : (
        <IconButton icon={Trash2} label={`Remove ${d}`} isDisabled={!canManage || save.isPending} onPress={() => commit(domains.filter((x) => x !== d))} />
      )}
    </div>
  );
  return (
    <Card>
      <CardHeader title="Domains" description="Point a CNAME at the default address, then add the hostname here. A certificate is issued automatically." />
      {item(ws.host, true)}
      {domains.map((d) => item(d, false))}
      <div className="flex items-end gap-3 px-5 py-4">
        <div className="flex-1"><TextField label="Hostname" value={host} onChange={setHost} placeholder="dev.example.com" mono /></div>
        <Button
          variant="secondary"
          isDisabled={!canManage || !valid || save.isPending}
          tooltip={!canManage ? "Needs the owner or admin role" : `Serve this workspace at ${h || "your hostname"}`}
          onPress={() => { commit([...domains, h]); setHost(""); }}
        >
          <Plus className="size-4" /> Add domain
        </Button>
      </div>
    </Card>
  );
}
