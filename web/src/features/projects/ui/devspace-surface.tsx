import type { AgentView } from "../../../lib/api/build-types";
import type { Project } from "../../../lib/api/project-types";
import type { Workspace } from "../../../lib/api/types";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { AppCards } from "../../apps/ui/app-cards";
import { useWsSession } from "../../../lib/hooks/use-ws-session";
import { BuildSurface } from "./build-surface";
import { DevspaceHome } from "./devspace-home";
import { DevspaceSignIn } from "./devspace-signin";
import type { Surface } from "./devspace-rail";
import { RRFlowSurface } from "./rrflow-surface";

type Props = { tenantId: string; surface: Surface; ws?: Workspace; project: Project; agent?: AgentView };

/**
 * Renders the selected surface. The engine view (the workspace's VS Code server) stays mounted
 * while the devspace is open, so Code and Terminal share one live session and switching
 * surfaces never reloads it. ⚠ M1 interim: Code and Terminal show the engine itself; the
 * decomposed editor, file tree and true TTY (decision 2: WezTerm + Zellij) need the in-workspace
 * agent's tty/files endpoints (02b "Agent and access"), which are not built yet.
 */
export function DevspaceSurface({ tenantId, surface, ws, project, agent }: Props) {
  const [appsOpen, setAppsOpen] = useState(false);
  const running = ws?.state === "running";
  const session = useWsSession(running ? ws.url : undefined);
  const ready = running && session.state === "ok";
  const engine = surface === "code" || surface === "terminal";
  return (
    <div className="relative min-w-0 flex-1 overflow-auto">
      {ready && (
        <iframe
          title={`${project.name} engine`}
          src={ws.url}
          className={engine ? "absolute inset-0 size-full border-0" : "hidden"}
          allow="clipboard-read; clipboard-write"
        />
      )}
      {surface === "terminal" && (
        <div className="absolute inset-x-0 bottom-0 z-20 border-t border-line bg-surface">
          <div className="px-3 py-2">
            <Button size="sm" variant="ghost" tooltip="Show the official install commands for this terminal" onPress={() => setAppsOpen((v) => !v)}>
              {appsOpen ? "Hide install commands" : "Install an app"}
            </Button>
          </div>
          {appsOpen && (
            <div className="max-h-64 overflow-auto px-4 pb-3">
              <p className="mb-2 text-xs text-fg-muted">Copy a command and paste it into this terminal. It installs the app and starts the vendor sign-in.</p>
              <AppCards tenantId={tenantId} />
            </div>
          )}
        </div>
      )}
      {engine && running && !ready && <DevspaceSignIn onSignIn={session.signIn} checking={session.state === "checking"} />}
      {engine && !running && (
        <div className="p-8"><EmptyState title="Starting your devspace" description="Your files and open editors come back exactly as you left them." /></div>
      )}
      {surface === "home" && <DevspaceHome ws={ws} project={project} />}
      {surface === "build" && (
        <div className="absolute inset-0 z-10 bg-surface">
          <BuildSurface tenantId={tenantId} projectId={project.id} view={agent} ws={ws} ready={ready} onSignIn={session.signIn} />
        </div>
      )}
      {surface === "rrflow" && <RRFlowSurface tenantId={tenantId} projectId={project.id} ws={ws} />}
      {surface === "previews" && (
        <div className="p-6">
          {running ? (
            <a className="text-primary underline" href={`https://${ws.host}`} target="_blank" rel="noopener">{ws.host}</a>
          ) : (
            <EmptyState title="No previews yet" description="Start your app and its address shows up here." />
          )}
        </div>
      )}
      {surface === "files" && (
        <div className="p-8"><EmptyState title="Files are coming soon" description="Until then, use Code to browse and edit your files." /></div>
      )}
    </div>
  );
}
