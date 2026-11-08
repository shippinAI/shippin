import { useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "../../../lib/router/router";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { useAgent } from "../../../lib/hooks/use-build";
import { devspaceOf, useOpenDevspace } from "../../../lib/hooks/use-devspace";
import { lastProject, useProject, useProjects } from "../../../lib/hooks/use-projects";
import { useWorkspaces } from "../../../lib/hooks/use-workspaces";
import { DevspaceBar } from "./devspace-bar";
import { DevspaceJob } from "./devspace-job";
import { DevspaceRail, surfaceKey, type Surface } from "./devspace-rail";
import { DevspaceSurface } from "./devspace-surface";

/**
 * The project devspace (M1): Shippin's own surfaces (Home, Code, Terminal, Previews, Files) over
 * the workspace's VS Code server, which runs in the background as the engine (02b "IDE runs on
 * 127.0.0.1 ... exposed only through the agent/relay"; CONSOLE-IA Workspaces detail tabs).
 * Opening a project is one click: the devspace is created or started automatically, and the
 * last surface per project is restored so switching projects resumes where you left off.
 */
export function DevspacePage() {
  const { tenantId = "", projectId = "" } = useParams();
  const nav = useNavigate();
  const projects = useProjects(tenantId);
  const project = useProject(tenantId, projectId);
  const list = useWorkspaces(tenantId);
  const open = useOpenDevspace(tenantId);
  const agent = useAgent(tenantId, projectId);
  const built = !!agent.data?.run; // prompt builds (M4): the build job creates the devspace itself
  const [surface, setSurface] = useState<Surface>(() => (localStorage.getItem(surfaceKey(projectId)) as Surface) || "home");
  const opened = useRef(""); // project whose devspace this page already opened (one per switch)
  const env = project.data?.environments[0];
  const ws = devspaceOf(list.data?.items, projectId, env?.id);

  useEffect(() => {
    setSurface((localStorage.getItem(surfaceKey(projectId)) as Surface) || "home");
    lastProject.set(tenantId, projectId);
  }, [tenantId, projectId]);
  useEffect(() => localStorage.setItem(surfaceKey(projectId), surface), [projectId, surface]);
  useEffect(() => {
    // One click: arriving here with no devspace (or a stopped one) starts it.
    if (!project.data || project.data.id !== projectId || !env || !list.data || opened.current === projectId) return;
    if (project.data.state !== "ready") return;
    if (!agent.isSuccess || (built && !ws)) return;
    if (ws && ws.state !== "stopped") return;
    opened.current = projectId;
    open.mutate({ project: project.data, env });
  }, [projectId, project.data, env, list.data, ws, open, agent.isSuccess, built]);

  if (project.isPending) return <Skeleton className="h-[70vh]" />;
  if (!project.data) return <p className="text-sm text-fg-muted">Project not found.</p>;
  return (
    <div className="flex h-[calc(100vh-7rem)] flex-col overflow-hidden rounded-xl border border-line bg-surface shadow-[var(--shadow-e2)]">
      <DevspaceBar
        project={project.data}
        projects={projects.data ?? []}
        ws={ws}
        onSwitch={(id) => nav(`/t/${tenantId}/projects/${id}`)}
      />
      <DevspaceJob tenantId={tenantId} jobId={open.data?.job?.id} />
      <div className="flex min-h-0 flex-1">
        <DevspaceRail value={surface} onChange={setSurface} />
        <DevspaceSurface key={projectId} tenantId={tenantId} surface={surface} ws={ws} project={project.data} agent={agent.data} />
      </div>
    </div>
  );
}
