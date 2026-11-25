import { MoreHorizontal, Play, RotateCw, Square, Trash2 } from "lucide-react";
import { useState } from "react";
import { Button } from "react-aria-components";
import { ConfirmDialog } from "../../../../lib/design/components/dialog/confirm-dialog";
import { Menu, MenuItem } from "../../../../lib/design/components/menu/menu";
import { Tooltip } from "../../../../lib/design/components/tooltip/tooltip";
import type { Workspace } from "../../../../lib/api/types";
import { type WsAction, useWorkspaceAction } from "../../../../lib/hooks/use-workspaces";

const icons: Record<string, typeof Play> = { start: Play, stop: Square, restart: RotateCw, delete: Trash2 };
const verbOf = (id: string) => id.slice(id.lastIndexOf(".") + 1) as WsAction;

/**
 * The actions the server offers on a development space, drawn as it lists them: which are there,
 * which can be used now and why not are the server's. One marked destructive asks for the name first.
 */
export function WorkspaceMenu({ ws }: { ws: Workspace }) {
  const act = useWorkspaceAction(ws.tenant_id);
  const [confirm, setConfirm] = useState<WsAction>();
  const offers = ws.actions;
  const locked = !offers.some((o) => o.enabled);
  const why = locked ? (offers[0]?.why ?? "Nothing can be done to it now") : "Power and lifecycle";
  return (
    <>
      <Menu
        label={`Actions for ${ws.name}`}
        trigger={
          <Tooltip content={why}>
            <Button
              aria-label={`Actions for ${ws.name}`}
              isDisabled={locked}
              className="focus-ring inline-flex size-8 cursor-default items-center justify-center rounded-lg border border-line bg-raised text-fg-muted data-hovered:text-fg data-disabled:opacity-40"
            >
              <MoreHorizontal className="size-4" />
            </Button>
          </Tooltip>
        }
      >
        {offers.map((o) => {
          const verb = verbOf(o.id);
          const Icon = icons[verb] ?? MoreHorizontal;
          return (
            <MenuItem
              key={o.id}
              id={o.id}
              isDisabled={!o.enabled}
              onAction={() => (o.destructive ? setConfirm(verb) : act.mutate({ id: ws.id, name: ws.name, action: verb }))}
            >
              <Icon className={o.destructive ? "size-4 text-danger" : "size-4"} />{" "}
              <span className={o.destructive ? "text-danger" : undefined}>{o.destructive ? `${o.label}…` : o.label}</span>
            </MenuItem>
          );
        })}
      </Menu>
      <ConfirmDialog
        isOpen={confirm !== undefined}
        onOpenChange={(open) => !open && setConfirm(undefined)}
        title={`Delete ${ws.name}?`}
        body={<p>This permanently deletes the workspace, its home volume and its HTTPS address. Files in the workspace are lost.</p>}
        typeToConfirm={ws.name}
        confirmLabel="Delete workspace"
        onConfirm={() => confirm && act.mutate({ id: ws.id, name: ws.name, action: confirm, confirm: ws.name })}
      />
    </>
  );
}
