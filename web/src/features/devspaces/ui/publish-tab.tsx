import { ExternalLink, Globe, RotateCcw, Undo2 } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { HelpPopover } from "../../../lib/design/components/popover/help-popover";
import { TextField } from "../../../lib/design/components/field/text-field";
import type { Workspace } from "../../../lib/api/types";
import { timeAgo } from "../../../lib/format";
import { useListening } from "../../../lib/hooks/use-previews";
import { siteSlug, useSite, useSiteAction } from "../../../lib/hooks/use-site";

/** One-click Publish: a frozen release of the app at a public HTTPS URL anyone can open,
 *  with Unpublish and Rollback. Editing the devspace never changes the live site. */
export function PublishTab({ ws, canEdit }: { ws: Workspace; canEdit: boolean }) {
  const [slug, setSlug] = useState(siteSlug(ws.name));
  const listening = useListening(ws.tenant_id, ws.id, ws.state === "running").data;
  const [port, setPort] = useState("");
  const [folder, setFolder] = useState("/home/dev/code");
  const p = Number(port || listening?.items[0]?.port || 0);
  const site = useSite(ws.tenant_id, ws.id, slug).data;
  const act = useSiteAction(ws.tenant_id, ws.id);
  const off = !canEdit || act.isPending;
  const prev = site?.releases.find((_r, i) => i > 0 && site.releases[i - 1]?.id === site.current);
  return (
    <div className="space-y-5">
      <Card>
        <CardHeader title="Publish" description="Shippin takes a snapshot of this devspace and serves it at a public address. Keep editing; publish again when you're ready."
          actions={
            <HelpPopover title="How publishing works" placement="left">
              <p>1. The app folder is scanned for secrets; publishing stops if any are found.</p>
              <p>2. The devspace is snapshotted and served at <span className="font-mono">site-name</span> over HTTPS on the app port.</p>
              <p>3. Each publish is a release. Roll back or unpublish any time; the last 10 are kept.</p>
            </HelpPopover>
          } />
        <div className="grid gap-4 p-5 sm:grid-cols-[1fr_8rem_auto] sm:items-end">
          <TextField label="Site name" value={slug} onChange={(v) => setSlug(siteSlug(v))} isDisabled={off} />
          <TextField label="App folder" value={folder} onChange={setFolder} isDisabled={off} />
          <TextField label="App port" value={port} placeholder={String(listening?.items[0]?.port ?? "5173")} onChange={setPort} isDisabled={off} />
          <Button variant="primary" tooltip="Scan the app folder for secrets, then snapshot the devspace and put it on the web" isDisabled={off || ws.state !== "running" || !p}
            isPending={act.isPending && act.variables?.action === "publish"} onPress={() => act.mutate({ action: "publish", slug, port: p, path: folder })}>
            <Globe className="size-4" /> {site?.published ? "Publish update" : "Publish"}
          </Button>
        </div>
        {site?.published && site.url && (
          <div className="flex flex-wrap items-center gap-3 border-t border-line px-5 py-3">
            <a className="inline-flex items-center gap-1 font-mono text-sm text-primary underline" href={site.url} target="_blank" rel="noreferrer">
              {site.url} <ExternalLink className="size-3.5" />
            </a>
            <span className="text-xs text-fg-muted">Release {site.current}{site.rrflow_recorded ? " · logged in rrflow" : ""}</span>
            <span className="flex-1" />
            <Button size="sm" variant="secondary" tooltip="Serve the previous release" isDisabled={off || !prev || !p}
              onPress={() => act.mutate({ action: "rollback", slug, port: p })}>
              <Undo2 className="size-4" /> Roll back
            </Button>
            <Button size="sm" variant="ghost" tooltip="Take the site offline; releases are kept" isDisabled={off}
              onPress={() => act.mutate({ action: "unpublish", slug, port: p })}>
              Unpublish
            </Button>
          </div>
        )}
      </Card>
      <Card>
        <CardHeader title="Releases" description="The last 10 releases are kept. Roll back to any of them." />
        <ul className="divide-y divide-line">
          {(site?.releases ?? []).map((r) => (
            <li key={r.id} className="flex items-center gap-4 px-5 py-3 text-sm">
              <span className="font-mono text-fg">{r.id}</span>
              <span className="text-fg-muted">{timeAgo(r.at)}</span>
              {r.id === site?.current && <span className="text-xs text-primary">live</span>}
              <span className="flex-1" />
              {r.id !== site?.current && (
                <Button size="sm" variant="ghost" tooltip={`Serve ${r.id}`} isDisabled={off || !p}
                  onPress={() => act.mutate({ action: "rollback", slug, port: p, release: r.id })}>
                  <RotateCcw className="size-4" /> Use
                </Button>
              )}
            </li>
          ))}
          {!site?.releases.length && <li className="px-5 py-3 text-sm text-fg-muted">{site?.note ?? "Nothing published yet."}</li>}
        </ul>
      </Card>
    </div>
  );
}
