import { Code2 } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Select } from "../../../lib/design/components/select/select";
import type { Project } from "../../../lib/api/project-types";
import type { Workspace } from "../../../lib/api/types";
import { StateBadge } from "../../workspaces/ui/state-badge";
import { AIStatusChip } from "./ai-status-chip";

type Props = { project: Project; projects: Project[]; ws?: Workspace; onSwitch: (id: string) => void };

/** Devspace header: project switcher, environment, state, and the optional advanced engine view. */
export function DevspaceBar({ project, projects, ws, onSwitch }: Props) {
  const ready = ws?.state === "running";
  return (
    <div className="flex items-end gap-4 border-b border-line px-4 py-3">
      <div className="w-56">
        <Select label="Project" value={project.id} onChange={onSwitch}
          options={projects.filter((p) => p.state === "ready").map((p) => ({ id: p.id, label: p.name }))} />
      </div>
      <div className="flex items-center gap-2 pb-2">
        {project.environments.slice(0, 1).map((e) => <Badge key={e.id} tone="info">{e.name}</Badge>)}
        {ws ? <StateBadge state={ws.state} /> : <Badge>Preparing…</Badge>}
      </div>
      <div className="ml-auto flex items-center gap-1 pb-0.5">
        <AIStatusChip />
        <Button variant="ghost" size="sm" isDisabled={!ready}
          tooltip={ready ? "Advanced: open the full VS Code view in a new tab" : "Available once the devspace is running"}
          onPress={() => ws && window.open(ws.url, "_blank", "noopener")}>
          <Code2 className="size-4" /> Advanced: VS Code
        </Button>
      </div>
    </div>
  );
}
