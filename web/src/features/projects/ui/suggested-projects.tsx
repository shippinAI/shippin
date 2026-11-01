import { FolderSearch, GitBranch, Package } from "lucide-react";
import { GridList, GridListItem } from "react-aria-components";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { useAdopt, useDiscover, type FoundFolder } from "../../../lib/hooks/use-discover";
import { useWorkspaces } from "../../../lib/hooks/use-workspaces";
import { PublishFolder } from "./publish-folder";

/** Folders found in the devspace that look like projects; one click adopts one (no name asked). */
export function SuggestedProjects({ tenantId }: { tenantId: string }) {
  const ws = useWorkspaces(tenantId).data?.items.find((w) => w.state === "running");
  const q = useDiscover(tenantId, ws?.id);
  const fresh = (q.data?.items ?? []).filter((f) => !f.adopted);
  const unsynced = (q.data?.items ?? []).filter((f) => f.adopted && !f.remote && f.project_id);
  if (!ws) return null;
  return (
    <Card elevation={2}>
      <CardHeader title="Suggested from your devspace" description={`Found in ${ws.name}. Adopt one and it becomes a project, named after its folder.`} />
      <div className="px-5 pb-5">
        {q.isPending && <Skeleton className="h-16" />}
        {q.data && fresh.length === 0 && (
          <p className="flex items-center gap-2 text-sm text-fg-subtle">
            <FolderSearch className="size-4" /> {q.data.note ?? "No new project folders found."}
          </p>
        )}
        {fresh.length > 0 && (
          <GridList aria-label="Suggested projects" items={fresh} className="divide-y divide-line">
            {(f) => <SuggestedRow key={f.path} id={f.path} f={f} tenantId={tenantId} workspaceId={ws.id} />}
          </GridList>
        )}
        {unsynced.length > 0 && (
          <div className="mt-4 border-t border-line pt-3">
            <p className="mb-2 text-xs font-medium uppercase tracking-wide text-fg-subtle">Not on GitHub yet</p>
            {unsynced.map((f) => (
              <div key={f.path} className="flex items-center gap-3 py-2">
                <span className="min-w-0 flex-1 truncate font-mono text-sm text-fg">{f.slug}</span>
                <PublishFolder tenantId={tenantId} f={f} />
              </div>
            ))}
          </div>
        )}
      </div>
    </Card>
  );
}

function SuggestedRow({ f, id, tenantId, workspaceId }: { f: FoundFolder; id: string; tenantId: string; workspaceId: string }) {
  const adopt = useAdopt(tenantId, workspaceId);
  return (
    <GridListItem id={id} textValue={f.name} className="flex items-center gap-3 py-3 outline-none data-focus-visible:ring-2 data-focus-visible:ring-primary">
      <div className="min-w-0 flex-1">
        <div className="font-medium text-fg">{f.name}</div>
        <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-fg-subtle">
          <span className="font-mono">{f.path.replace("/home/dev/", "~/")}</span>
          {f.git ? <Badge><GitBranch className="size-3" /> git</Badge> : null}
          {f.manifests.map((m) => <Badge key={m}><Package className="size-3" /> {m}</Badge>)}
          {f.git && !f.remote && <Badge tone="warning">No GitHub remote</Badge>}
        </div>
      </div>
      <Button variant="primary" size="sm" tooltip={`Adopt as project “${f.slug}”`} isPending={adopt.isPending} onPress={() => adopt.mutate(f.path)}>
        Adopt
      </Button>
    </GridListItem>
  );
}
