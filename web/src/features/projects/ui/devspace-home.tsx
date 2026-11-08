import type { Project } from "../../../lib/api/project-types";
import type { Workspace } from "../../../lib/api/types";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { timeAgo } from "../../../lib/format";
import { ProvisionProgress } from "../../devspaces/ui/provision-progress";

/** Devspace home: state in plain words, and what "resume" keeps (02b Persistence). */
export function DevspaceHome({ ws, project }: { ws?: Workspace; project: Project }) {
  return (
    <div className="grid max-w-3xl gap-4 p-6">
      <Card elevation={2}>
        <CardHeader title={project.name} description={`Created ${timeAgo(project.created_at)}`} />
        <div className="space-y-2 px-5 pb-5 text-sm text-fg-muted">
          {!ws && <p>Setting up your devspace…</p>}
          {ws?.state === "provisioning" && <ProvisionProgress ws={ws} />}
          {ws?.state === "running" && <p>Your devspace is running. Pick Code or Terminal on the left to get going.</p>}
          {ws?.state === "stopped" && <p>Your devspace is paused. It wakes up when you open it.</p>}
          {ws?.state === "failed" && <p className="text-danger">Something went wrong: {ws.error}</p>}
        </div>
      </Card>
      <Card>
        <CardHeader title="Pick up where you left off" />
        <ul className="list-disc space-y-1 px-9 pb-5 text-sm text-fg-muted">
          <li>Your home folder lives on its own volume, so it's kept when the devspace stops or restarts.</li>
          <li>Open editors and terminals come back when you return to this project.</li>
          <li>Switch projects from the menu at the top; each one keeps its own state.</li>
        </ul>
      </Card>
    </div>
  );
}
