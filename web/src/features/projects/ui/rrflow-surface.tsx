import { Brain } from "lucide-react";
import type { Workspace } from "../../../lib/api/types";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { useAttachRRFlow, useServices } from "../../../lib/hooks/use-rrflow";
import { RRFlowViews } from "./rrflow-views";

type Props = { tenantId: string; projectId: string; ws?: Workspace };

/**
 * rrflow (02c-rrflow.md): rrflow/state — sessions, memory and the hash-chained decision log — as a
 * managed service on the tenant's private server, attached to this devspace. Read-only here
 * ("Persistence gets a read-only card"); agents write through the rrflow CLI in the devspace.
 */
export function RRFlowSurface({ tenantId, projectId, ws }: Props) {
  const services = useServices(tenantId, projectId);
  const attach = useAttachRRFlow(tenantId, projectId);
  const inst = services.data?.items.find((s) => s.kind === "rrflow-state");
  const bound = inst?.bindings.find((b) => b.workspace_id === ws?.id);
  if (services.data && !services.data.rrflow_enabled && !inst) {
    return <div className="p-8"><EmptyState title="rrflow isn't on here yet" description="Your organization hasn't switched rrflow on." /></div>;
  }
  return (
    <div className="space-y-4 p-5">
      <div className="flex flex-wrap items-center gap-3">
        <Brain className="size-5 text-primary" />
        <h2 className="text-base font-medium text-fg">rrflow state</h2>
        {inst && <Badge tone={inst.state === "ready" ? "success" : inst.state === "error" ? "danger" : "info"}>{inst.state}</Badge>}
        {bound && <Badge tone={bound.state === "bound" ? "success" : bound.state === "error" ? "danger" : "neutral"}>{bound.state === "bound" ? "attached to this devspace" : bound.state}</Badge>}
        {(!inst || !bound || bound.state === "error" || inst.state === "error") && (
          <Button size="sm" variant="primary" isDisabled={attach.isPending || ws?.state !== "running"} onPress={() => attach.mutate(ws?.id)}
            tooltip={ws?.state === "running" ? "Run rrflow on your private server and connect it here" : "Start the devspace first"}>
            {inst ? "Retry attach" : "Attach rrflow"}
          </Button>
        )}
      </div>
      {(inst?.error || bound?.error) && <p className="text-sm text-danger">{inst?.error || bound?.error}</p>}
      {!inst && (
        <p className="max-w-prose text-sm text-fg-muted">
          rrflow keeps your agents' sessions, what they've learned about this project, and every decision in a tamper-evident log. It runs on your own private server; nothing leaves it.
        </p>
      )}
      {inst?.state === "ready" && <RRFlowViews tenantId={tenantId} projectId={projectId} serviceId={inst.id} />}
    </div>
  );
}
