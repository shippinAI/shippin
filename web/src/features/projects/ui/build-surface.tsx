import { Play, RotateCcw } from "lucide-react";
import type { AgentView } from "../../../lib/api/build-types";
import type { Workspace } from "../../../lib/api/types";
import { Button } from "../../../lib/design/components/button/button";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { useStartAgent } from "../../../lib/hooks/use-build";
import { BuildSignIn } from "./build-signin";
import { BuildSteps } from "./build-steps";

type Props = { tenantId: string; projectId: string; view?: AgentView; ws?: Workspace; ready: boolean; onSignIn: () => void };

/**
 * Build (M4): Shippin's own view of the prompt build. Steps and plain status first; the agent's
 * live terminal (ttyd on the devspace, same sign-in as the IDE) below so you can watch or steer.
 * VS Code keeps running in the background as the engine; Code opens it.
 */
export function BuildSurface({ tenantId, projectId, view, ws, ready, onSignIn }: Props) {
  const start = useStartAgent(tenantId, projectId);
  const run = view?.run;
  if (!run) return <div className="p-8"><EmptyState title="No build here" description="Projects created with Describe it show their build here." /></div>;
  const live = view.live;
  const label = view.provider_label;
  const canStart = view.mine && view.connected && (run.state === "waiting_ai" || run.state === "exited" || run.state === "failed");
  return (
    <div className="flex h-full min-h-0 flex-col gap-4 p-5">
      <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
        <div className="space-y-3">
          <p className="text-[13px] text-fg-muted">“{run.prompt_preview}”</p>
          <BuildSteps state={run.state} label={label} />
          {run.error && <p className="text-[13px] text-danger">{run.error}</p>}
        </div>
        <div className="space-y-3">
          {run.state === "waiting_ai" && view.mine && !view.connected && <BuildSignIn tenantId={tenantId} provider={run.provider} />}
          {run.state === "waiting_ai" && !view.mine && <p className="text-[13px] text-fg-muted">Waiting for the person who started this build to sign in.</p>}
          {live && (
            <p className="text-[13px] text-fg-muted" aria-live="polite">
              {live.exited ? `${label} finished (exit ${live.exit_code}).` : live.session ? `${label} is working.` : "Agent not running."}{" "}
              {live.files_changed > 0 && `${live.files_changed} file${live.files_changed === 1 ? "" : "s"} changed.`}
            </p>
          )}
          {canStart && (
            <Button size="sm" variant="primary" isDisabled={start.isPending} onPress={() => start.mutate(run.state === "exited")}
              tooltip={run.state === "exited" ? "Continue the last conversation" : "Start the build now"}>
              {run.state === "exited" ? <RotateCcw className="size-4" /> : <Play className="size-4" />}
              {run.state === "exited" ? "Keep going" : "Start build"}
            </Button>
          )}
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-hidden rounded-[var(--radius-card)] border border-line bg-canvas">
        {view.term_url && ready && (run.state === "running" || run.state === "exited" || run.state === "starting") ? (
          <iframe title={`${label} live terminal`} src={view.term_url} className="size-full border-0" allow="clipboard-read; clipboard-write" />
        ) : ws?.state === "running" && !ready ? (
          <div className="p-6"><Button size="sm" onPress={onSignIn} tooltip="Sign in to your devspace to watch the agent">Open the live terminal</Button></div>
        ) : (
          <div className="p-6"><EmptyState title="Live terminal" description="Appears here when your agent starts. You can type in it to steer." /></div>
        )}
      </div>
    </div>
  );
}
