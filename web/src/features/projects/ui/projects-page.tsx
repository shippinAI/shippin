import { FolderGit2, Play } from "lucide-react";
import { useNavigate } from "../../../lib/router/router";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card } from "../../../lib/design/components/card/card";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import type { Project } from "../../../lib/api/project-types";
import { lastProject, useProjects } from "../../../lib/hooks/use-projects";
import { useTenant } from "../../../lib/hooks/use-tenant";
import { PageHeader } from "../../../template/page-header";
import { NewProjectPanel } from "./new-project-panel";
import { ProjectUsage } from "./project-usage";
import { SuggestedProjects } from "./suggested-projects";

/** Projects list (M1): one click on a project opens its devspace; the last one is offered first. */
export function ProjectsPage() {
  const { tenant } = useTenant();
  const t = tenant?.tenant_id ?? "";
  const q = useProjects(t);
  const nav = useNavigate();
  const last = lastProject.get(t);
  const open = (p: Project) => nav(`/t/${t}/projects/${p.id}`);
  const items = [...(q.data ?? [])].sort((a, b) => Number(b.id === last) - Number(a.id === last));
  return (
    <>
      <PageHeader eyebrow={tenant?.name} title="Projects" description="Each project gets its own devspace: code, terminal and previews in one place. Pick up right where you left off." />
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1.6fr_1fr]">
        <div className="space-y-3">
          <SuggestedProjects tenantId={t} />
          {q.isPending && Array.from({ length: 2 }, (_, i) => <Skeleton key={i} className="h-20" />)}
          {q.data?.length === 0 && <Card><EmptyState title="No projects yet" description="Create your first project to get a devspace." /></Card>}
          {items.map((p) => (
            <Card key={p.id} elevation={2} className="flex items-center gap-4 p-4">
              <FolderGit2 className="size-5 text-hue-workspaces" />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 font-medium text-fg">
                  {p.name}
                  {p.id === last && <Badge tone="primary">Last opened</Badge>}
                </div>
                <div className="mt-1 flex flex-wrap gap-1.5 text-xs text-fg-subtle">
                  {p.environments.map((e) => <Badge key={e.id}>{e.name}</Badge>)}
                  <span>{p.workspaces} devspace{p.workspaces === 1 ? "" : "s"}</span>
                </div>
              </div>
              {p.state === "ready" ? (
                <Button variant="primary" size="sm" tooltip={`Open the ${p.name} devspace`} onPress={() => open(p)}>
                  <Play className="size-4" /> {p.id === last ? "Resume" : "Open"}
                </Button>
              ) : (
                <Badge tone="warning">Waiting for your private server</Badge>
              )}
            </Card>
          ))}
        </div>
        <div className="space-y-6">
          <NewProjectPanel tenantId={t} />
          <div id="usage"><ProjectUsage tenantId={t} projects={q.data ?? []} /></div>
        </div>
      </div>
    </>
  );
}
