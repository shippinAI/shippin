import { Bot, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { CodeBlock } from "../../../lib/design/components/code-block/code-block";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import { timeAgo } from "../../../lib/format";
import { type AccessKey, type CreatedAccessKey, useAccessKeys, useCreateAccessKey, useRevokeAccessKey } from "../../../lib/hooks/use-access-keys";
import { PageHeader } from "../../../template/page-header";

const lasts = [
  { id: "7", label: "7 days" },
  { id: "30", label: "30 days" },
  { id: "90", label: "90 days" },
];

/** Access keys let an AI client act as you through MCP, with your permissions, audit and events. */
export function AccessKeysPage() {
  const q = useAccessKeys();
  const create = useCreateAccessKey();
  const revoke = useRevokeAccessKey();
  const [name, setName] = useState("");
  const [days, setDays] = useState("30");
  const [made, setMade] = useState<CreatedAccessKey | null>(null);
  const [target, setTarget] = useState<AccessKey | null>(null);
  const items = q.data?.items ?? [];
  const cols: ColumnDef<AccessKey>[] = [
    { id: "name", label: "Name", isRowHeader: true, render: (k) => <span className="flex items-center gap-2 font-medium text-fg"><Bot className="size-4 text-hue-ai" />{k.name}</span> },
    { id: "prefix", label: "Starts with", render: (k) => <span className="font-mono text-sm">{k.prefix}…</span> },
    { id: "used", label: "Last used", render: (k) => (k.last_used_at ? timeAgo(k.last_used_at) : "Never") },
    { id: "expires", label: "Expires", render: (k) => new Date(k.expires_at).toLocaleDateString() },
    { id: "x", label: "", width: "60px", render: (k) => <IconButton icon={Trash2} label={`Revoke key ${k.name}`} onPress={() => setTarget(k)} /> },
  ];
  const mcp = `${window.location.origin}/v1/mcp`;
  return (
    <>
      <PageHeader
        eyebrow="Account"
        title="AI access"
        description="An access key lets an AI app such as Claude act as you here, through MCP. It can do what you can do, every change it makes is in the audit log under your name, and you can revoke it at any time."
      />
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1.5fr_1fr]">
        <Card elevation={2} className="overflow-hidden">
          <CardHeader title={`${items.length} key${items.length === 1 ? "" : "s"}`} />
          <DataTable
            label="Access keys"
            columns={cols}
            rows={items}
            empty={<EmptyState title="No access keys" description="Make one for each AI app or script, so you can revoke them one at a time." />}
          />
        </Card>
        <Card>
          <CardHeader title="Make a key" description="Name it after what will use it." />
          <div className="space-y-4 p-5">
            <TextField label="Name" value={name} onChange={setName} placeholder="Claude on my laptop" />
            <Select label="Lasts" options={lasts} value={days} onChange={setDays} />
            <div className="flex justify-end">
              <Button
                variant="primary"
                size="sm"
                isDisabled={!name.trim() || create.isPending}
                tooltip={name.trim() ? "Make the key and show it once" : "Name the key first"}
                onPress={() => create.mutate({ name: name.trim(), days: Number(days) }, { onSuccess: (k) => { setMade(k); setName(""); } })}
              >
                <Plus className="size-4" /> Make key
              </Button>
            </div>
          </div>
        </Card>
      </div>
      {made && (
        <Card elevation={2} className="mt-6">
          <CardHeader
            title={`Your key "${made.name}"`}
            description="Copy it now: it is shown this once. Anyone with it can act as you until it expires or you revoke it."
          />
          <div className="space-y-4 p-5">
            <CodeBlock label="Access key" code={made.token} />
            <CodeBlock label="Add to Claude Code" code={`claude mcp add --transport http shippin ${mcp} --header "Authorization: Bearer ${made.token}"`} />
            <CodeBlock
              label="MCP client settings"
              code={JSON.stringify({ mcpServers: { shippin: { type: "http", url: mcp, headers: { Authorization: `Bearer ${made.token}` } } } }, null, 2)}
            />
            <div className="flex justify-end">
              <Button variant="secondary" size="sm" tooltip="Hide the key; it can't be shown again" onPress={() => setMade(null)}>
                I have saved it
              </Button>
            </div>
          </div>
        </Card>
      )}
      <ConfirmDialog
        isOpen={!!target}
        onOpenChange={(o) => !o && setTarget(null)}
        title={`Revoke key "${target?.name}"?`}
        body={<p>Whatever uses this key stops working at once. Make a new key to reconnect it.</p>}
        confirmLabel="Revoke key"
        onConfirm={() => target && revoke.mutate(target)}
      />
    </>
  );
}
