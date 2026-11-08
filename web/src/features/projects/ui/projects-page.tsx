import { FolderGit2, Play } from "lucide-react";
import { useNavigate, useParams } from "../../../lib/router/router";
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
import { useOrganizationWorkspaces } from "../../workspaces/use-organization-workspaces";
import { useText } from "../../../lib/ui-config/use-ui-config";

/** Projects list (M1): one click on a project opens its devspace; the last one is offered first. */
export function ProjectsPage() {
  const { tenant } = useTenant();
  const t = tenant?.tenant_id ?? "";
  const { groupId } = useParams();
  const groups = useOrganizationWorkspaces(t);
  const group = groups.data?.find((g) => g.id === groupId);
  const q = useProjects(t, groupId);
  const text = useText();
  const nav = useNavigate();
  const last = lastProject.get(t);
  const open = (p: Project) => nav(`/t/${t}/projects/${p.id}`);
  const items = [...(q.data ?? [])].sort((a, b) => Number(b.id === last) - Number(a.id === last));
  return (
    <>
      <PageHeader eyebrow={tenant?.name} title={group?.name ?? "Projects"} description={text("spaces.projectDescription")} />
      <Button className="mb-4" variant="ghost" size="sm" tooltip={text("spaces.back")} onPress={() => nav(`/t/${t}/workspaces`)}>{text("spaces.back")}</Button>
      {q.isError && <p role="alert" className="mb-4 text-danger">{q.error.message}</p>}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1.6fr_1fr]">
        <div className="space-y-3">
          {!groupId && <SuggestedProjects tenantId={t} />}
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
              {p.state !== "ready" && <Badge tone="warning">{text("spaces.pending")}</Badge>}
                <Button variant="primary" size="sm" tooltip={`Open the ${p.name} devspace`} onPress={() => open(p)}>
                  <Play className="size-4" /> {p.id === last ? "Resume" : "Open"}
                </Button>
            </Card>
          ))}
        </div>
        <div className="space-y-6">
          {tenant?.can_manage && tenant.roles.length > 0 && (!groupId || group) && <NewProjectPanel key={groupId ?? "all"} tenantId={t} workspaceId={groupId} />}
          <div id="usage"><ProjectUsage tenantId={t} projects={q.data ?? []} /></div>
        </div>
      </div>
    </>
  );
}
