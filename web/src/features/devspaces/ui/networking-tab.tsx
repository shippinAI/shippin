import { Plus, ShieldCheck, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import type { FirewallRule, Workspace } from "../../../lib/api/types";
import { useWorkspaceAccess } from "../../../lib/hooks/use-workspaces";


/** Inbound firewall rules, enforced on the private network. */
export function NetworkingTab({ ws, canManage }: { ws: Workspace; canManage: boolean }) {
  const save = useWorkspaceAccess(ws.tenant_id, ws.id, "firewall");
  const [rules, setRules] = useState<FirewallRule[]>(ws.firewall ?? []);
  const [proto, setProto] = useState<"tcp" | "udp">("tcp");
  const [port, setPort] = useState("");
  const [source, setSource] = useState("0.0.0.0/0");
  useEffect(() => setRules(ws.firewall ?? []), [ws.firewall]);
  const n = Number(port);
  const valid = Number.isInteger(n) && n >= 1 && n <= 65535 && n !== 22 && source.trim() !== "";
  const commit = (next: FirewallRule[]) => { setRules(next); save.mutate({ rules: next }); };
  const builtIn = [
    { what: "HTTPS (browser IDE)", port: "443", from: "Anyone, after sign-in" },
    { what: "SSH", port: String(ws.ssh_port), from: "Anyone with an account key" },
  ];
  const row = "grid grid-cols-[1.4fr_0.7fr_0.6fr_1.4fr_40px] items-center gap-3 px-5 py-3 text-sm";
  return (
    <Card>
      <CardHeader title="Inbound rules" description="Everything not listed here is blocked. Changes apply within seconds." />
      <div className={`${row} border-b border-line text-[11.5px] font-medium tracking-wide text-fg-subtle uppercase`}>
        <span>Type</span><span>Protocol</span><span>Port</span><span>Sources</span><span />
      </div>
      {builtIn.map((b) => (
        <div key={b.port} className={`${row} border-b border-line text-fg-muted`}>
          <span className="flex items-center gap-2"><ShieldCheck className="size-4 text-success" />{b.what}</span>
          <span>TCP</span><span className="font-mono">{b.port}</span><span>{b.from}</span><span />
        </div>
      ))}
      {rules.map((r, i) => (
        <div key={`${r.protocol}-${r.port}-${r.source}`} className={`${row} border-b border-line`}>
          <span>Custom</span><span className="uppercase">{r.protocol}</span>
          <span className="font-mono">{r.port}</span><span className="font-mono text-[13px]">{r.source}</span>
          <IconButton icon={Trash2} label={`Remove rule for port ${r.port}`} isDisabled={!canManage || save.isPending} onPress={() => commit(rules.filter((_, j) => j !== i))} />
        </div>
      ))}
      <div className="grid grid-cols-[0.8fr_0.8fr_1.6fr_auto] items-end gap-3 px-5 py-4">
        <Select label="Protocol" value={proto} onChange={(v) => setProto(v as "tcp" | "udp")} options={[{ id: "tcp", label: "TCP" }, { id: "udp", label: "UDP" }]} />
        <TextField label="Port" value={port} onChange={setPort} placeholder="8080" />
        <TextField label="Sources" value={source} onChange={setSource} placeholder="0.0.0.0/0 or 203.0.113.4/32" mono />
        <Button
          variant="secondary"
          isDisabled={!canManage || !valid || save.isPending}
          tooltip={!canManage ? "Needs the owner or admin role" : n === 22 ? "SSH is always open" : "Allow inbound traffic on this port"}
          onPress={() => { commit([...rules, { protocol: proto, port: n, source: source.trim() }]); setPort(""); }}
        >
          <Plus className="size-4" /> Add rule
        </Button>
      </div>
    </Card>
  );
}
