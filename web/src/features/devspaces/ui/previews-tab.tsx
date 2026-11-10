import { ExternalLink, Plus, Radio as RadioIcon, Trash2 } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Switch } from "../../../lib/design/components/switch/switch";
import type { Preview, Workspace } from "../../../lib/api/types";
import { useListening, useSetPreviews } from "../../../lib/hooks/use-previews";

/** Per-port preview URLs: private (owner sign-in) by default, public per port. */
export function PreviewsTab({ ws, canEdit }: { ws: Workspace; canEdit: boolean }) {
  const set = useSetPreviews(ws.tenant_id, ws.id);
  const listening = useListening(ws.tenant_id, ws.id, ws.state === "running").data;
  const [port, setPort] = useState("");
  const cur: Preview[] = ws.preview_urls.map(({ port, public: pub }) => ({ port, public: pub }));
  const save = (previews: Preview[], message: string) => set.mutate({ previews, message });
  const live = new Set((listening?.items ?? []).map((l) => l.port));
  const suggest = (listening?.items ?? []).filter((l) => !cur.some((c) => c.port === l.port));
  const off = !canEdit || ws.state !== "running" || set.isPending;
  return (
    <div className="space-y-5">
      <Card>
        <CardHeader title="Preview URLs" description="Each port gets its own HTTPS address. Private previews need your sign-in; public ones are open to anyone with the link." />
        {ws.preview_urls.length === 0 && <EmptyState title="No previews yet" description="Start a dev server in the workspace, then add its port here." />}
        <ul className="divide-y divide-line">
          {ws.preview_urls.map((p) => (
            <li key={p.port} className="flex items-center gap-4 px-5 py-3">
              <span className="w-16 font-mono text-sm text-fg">:{p.port}</span>
              <span className={`inline-flex items-center gap-1.5 text-xs ${live.has(p.port) ? "text-success" : "text-fg-subtle"}`}>
                <RadioIcon className="size-3.5" /> {live.has(p.port) ? "Listening" : "Not listening"}
              </span>
              <a href={p.url} target="_blank" rel="noreferrer" className="min-w-0 flex-1 truncate font-mono text-sm text-primary hover:underline">{p.url}</a>
              <Switch
                isSelected={p.public}
                isDisabled={off}
                tooltip={p.public ? "Anyone with the link can open it" : "Only you (signed in) can open it"}
                onChange={(v) => save(cur.map((c) => (c.port === p.port ? { ...c, public: v } : c)), `:${p.port} is now ${v ? "public" : "private"}`)}
              >
                {p.public ? "Public" : "Private"}
              </Switch>
              <Button size="sm" variant="secondary" tooltip={`Open ${p.url} in a new tab`} onPress={() => window.open(p.url, "_blank", "noopener")}>
                <ExternalLink className="size-3.5" /> Open
              </Button>
              <IconButton icon={Trash2} label={`Remove preview :${p.port}`} isDisabled={off} onPress={() => save(cur.filter((c) => c.port !== p.port), `Preview :${p.port} removed`)} />
            </li>
          ))}
        </ul>
      </Card>
      <Card>
        <CardHeader title="Add a preview" description={listening?.note ?? "Detected ports update every few seconds. Bind your server to 0.0.0.0 so the edge can reach it."} />
        <div className="flex flex-wrap items-end gap-3 px-5 py-4">
          {suggest.map((l) => (
            <Button key={l.port} size="sm" variant="secondary" isDisabled={off || l.local_only} tooltip={l.local_only ? "Bound to localhost only; restart it on 0.0.0.0" : `Publish :${l.port}`} onPress={() => save([...cur, { port: l.port, public: false }], `Preview :${l.port} added (private)`)}>
              <Plus className="size-3.5" /> :{l.port}{l.local_only ? " (localhost)" : ""}
            </Button>
          ))}
          <div className="w-40"><TextField label="Port" value={port} onChange={setPort} placeholder="5173" mono /></div>
          <Button size="sm" variant="primary" isDisabled={off || !/^\d{4,5}$/.test(port)} tooltip="Publish this port as a private preview" onPress={() => { save([...cur, { port: Number(port), public: false }], `Preview :${port} added (private)`); setPort(""); }}>
            <Plus className="size-3.5" /> Add preview
          </Button>
        </div>
      </Card>
    </div>
  );
}
