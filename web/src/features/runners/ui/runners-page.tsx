import { RunnerConfiguration } from "./runner-configuration";
import { RunnerResource } from "./runner-resource";
import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams } from "../../../lib/router/router";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { PageHeader } from "../../../template/page-header";
import { Select } from "../../../lib/design/components/select/select";
import { Tabs } from "../../../lib/design/components/tabs/tabs";
import { Card } from "../../../lib/design/components/card/card";
import { Button } from "../../../lib/design/components/button/button";
import { errorKey, loadProjects, runnerHref, runnerTab } from "../model";

export function RunnersPage() {
  const t = useText();
  const navigate = useNavigate();
  const { tenantId = "", projectId, tab } = useParams();
  const projects = useQuery({ queryKey: ["runner-projects", tenantId], queryFn: () => loadProjects(tenantId), enabled: !!tenantId, retry: false, gcTime: 0 });
  const project = !projects.isError ? projects.data?.find((p) => p.id === projectId) : undefined;
  return <>
    <PageHeader title={t("runners.title")} description={t("runners.description")} />
    {projects.isPending && <p role="status">{t("runners.loading")}</p>}
    {projects.isError && <div role="alert"><p>{t(errorKey(projects.error))}</p><Button tooltip={t("runners.refresh")} onPress={() => { void projects.refetch(); }}>{t("runners.refresh")}</Button></div>}
    {!projects.isError && projects.data && <div className="space-y-6">
      {!projects.data.length ? <p>{t("runners.noProjects")}</p> : <Select label={t("runners.project")} options={projects.data.map((p) => ({ id: p.id, label: p.name }))} value={project?.id ?? ""} onChange={(id) => navigate(runnerHref(tenantId, id, runnerTab(tab)))} />}
      {projectId && !project && <p role="alert">{t("runners.denied")}</p>}
      {!projectId && !!projects.data.length && <p>{t("runners.choose")}</p>}
      {project && <Card className="p-5"><Tabs label={t("runners.tabs")} selected={runnerTab(tab)} onSelect={(id) => navigate(runnerHref(tenantId, project.id, id))} tabs={[
        { id: "overview", label: t("runners.overview"), content: <div className="space-y-3"><h2 className="text-lg font-semibold">{project.name}</h2><p>{t("runners.overviewDescription")}</p><p className="text-fg-muted">{t("runners.capacityNotice")}</p></div> },
        { id: "runs", label: t("runners.runs"), content: <RunnerResource key={`${tenantId}:${project.id}:runs`} tenant={tenantId} project={project} resource="runs" /> },
        { id: "connections", label: t("runners.connections"), content: <RunnerResource key={`${tenantId}:${project.id}:connectors`} tenant={tenantId} project={project} resource="connectors" /> },
        { id: "configuration", label: t("runners.configuration"), content: <RunnerConfiguration key={`${tenantId}:${project.id}`} tenant={tenantId} project={project} /> },
      ]} /></Card>}
    </div>}
  </>;
}
