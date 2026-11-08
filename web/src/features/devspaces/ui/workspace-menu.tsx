import { MoreHorizontal, Play, RotateCw, Square, Trash2 } from "lucide-react";
import { useState } from "react";
import { Button } from "react-aria-components";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { Menu, MenuItem } from "../../../lib/design/components/menu/menu";
import { Tooltip } from "../../../lib/design/components/tooltip/tooltip";
import type { Workspace } from "../../../lib/api/types";
import { busy, useWorkspaceAction } from "../../../lib/hooks/use-workspaces";

/** Power and lifecycle actions; Delete asks for the workspace name. */
export function WorkspaceMenu({ ws, canManage }: { ws: Workspace; canManage: boolean }) {
  const act = useWorkspaceAction(ws.tenant_id);
  const [confirm, setConfirm] = useState(false);
  const locked = !canManage || busy(ws);
  const run = (action: "start" | "stop" | "restart") => act.mutate({ id: ws.id, name: ws.name, action });
  const running = ws.state === "running";
  const why = !canManage ? "Needs the owner or admin role" : busy(ws) ? "Wait for the current operation" : "Power and lifecycle";
  return (
    <>
      <Menu
        label={`Actions for ${ws.name}`}
        trigger={
          <Tooltip content={why}>
            <Button aria-label={`Actions for ${ws.name}`} isDisabled={locked} className="focus-ring inline-flex size-8 cursor-default items-center justify-center rounded-[var(--radius-control)] border border-line bg-raised text-fg-muted data-hovered:text-fg data-disabled:opacity-40">
              <MoreHorizontal className="size-4" />
            </Button>
          </Tooltip>
        }
      >
        <MenuItem id="start" isDisabled={running} onAction={() => run("start")}><Play className="size-4" /> Power on</MenuItem>
        <MenuItem id="stop" isDisabled={!running} onAction={() => run("stop")}><Square className="size-4" /> Power off</MenuItem>
        <MenuItem id="restart" isDisabled={!running} onAction={() => run("restart")}><RotateCw className="size-4" /> Restart</MenuItem>
        <MenuItem id="delete" onAction={() => setConfirm(true)}><Trash2 className="size-4 text-danger" /> <span className="text-danger">Delete…</span></MenuItem>
      </Menu>
      <ConfirmDialog
        isOpen={confirm}
        onOpenChange={setConfirm}
        title={`Delete ${ws.name}?`}
        body={<p>This permanently deletes the workspace, its home volume and its HTTPS address. Files in the workspace are lost.</p>}
        typeToConfirm={ws.name}
        confirmLabel="Delete workspace"
        onConfirm={() => act.mutate({ id: ws.id, name: ws.name, action: "delete" })}
      />
    </>
  );
}
