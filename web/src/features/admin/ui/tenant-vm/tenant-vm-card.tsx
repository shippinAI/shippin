import { Server } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { ConfirmDialog } from "../../../../lib/design/components/dialog/confirm-dialog";
import { Skeleton } from "../../../../lib/design/components/skeleton/skeleton";
import type { AdminTenant } from "../../../../lib/api/admin-types";
import { formatBytes } from "../../../../lib/format";
import { useTenantVM, useTenantVMAction } from "../../../../lib/hooks/use-tenant-vm";
import { StatusBadge } from "../status-badge";
import { TenantAppsPrep } from "./tenant-apps-prep";
import { VMContainers } from "./vm-containers";
import { VMMeter } from "./vm-meter";
import { sizeBytes } from "./vm-sizes";

/** Operator card for the tenant VM: state, CPU/RAM/disk, nested containers, lifecycle actions. */
export function TenantVMCard({ tenant: t }: { tenant: AdminTenant }) {
  const vm = useTenantVM(t.id);
  const act = useTenantVMAction();
  const [confirm, setConfirm] = useState<"stop" | "enable">();
  const v = vm.data;
  const run = (action: "start" | "stop" | "restart" | "enable") => act.mutate({ id: t.id, name: t.name, action });
  const busy = act.isPending || t.status === "deleted";
  return (
    <section className="mx-5 mt-2 rounded-lg border border-line bg-raised/50 px-4 py-3" aria-label="Tenant VM">
      <div className="flex items-center gap-2 text-[13px] font-medium text-fg">
        <Server className="size-4 text-fg-subtle" strokeWidth={1.75} />
        Tenant VM
        {v && <span className="ml-auto">{v.mode === "vm" ? <StatusBadge status={v.status} /> : <StatusBadge status="Legacy direct" />}</span>}
      </div>
      {vm.isLoading && <Skeleton className="mt-3 h-16 w-full" />}
      {vm.error && <p className="mt-2 text-xs text-danger">{vm.error.message}</p>}
      {v?.mode === "legacy" && (
        <div className="mt-2 space-y-3">
          <p className="text-xs text-fg-muted">Workspaces run as containers directly on the host. Enabling a VM sends only new workspaces into it; existing ones keep running where they are.</p>
          <Button size="sm" variant="secondary" isDisabled={busy} tooltip="Create this tenant's KVM VM with Incus inside" onPress={() => setConfirm("enable")}>Enable tenant VM</Button>
        </div>
      )}
      {v?.mode === "vm" && (
        <div className="mt-3 space-y-3">
          <div className="grid grid-cols-2 gap-x-5 gap-y-1 font-mono text-[12px] text-fg-muted">
            <span>{v.name}</span>
            <span className="text-right">{v.ipv4 || "—"} · inner {v.inner_subnet || "—"}</span>
          </div>
          <VMMeter label="vCPU" text={`${v.cpu_limit || "—"} vCPU`} />
          <VMMeter label="Memory" used={v.memory_used_bytes} total={v.memory_total_bytes || sizeBytes(v.memory_limit)} text={`${formatBytes(v.memory_used_bytes)} / ${v.memory_limit || "—"}`} />
          <VMMeter label="Workspace storage" used={v.data_used_bytes} total={v.data_total_bytes || sizeBytes(v.data_size)} text={`${formatBytes(v.data_used_bytes)} / ${v.data_size || "—"}`} />
          <p className="text-[11.5px] text-fg-subtle">Root disk {v.root_size || "—"} · encrypted tenant pool</p>
          <VMContainers items={v.containers ?? []} />
          <TenantAppsPrep id={t.id} />
          <div className="flex gap-2">
            {v.status === "Running" ? (
              <>
                <Button size="sm" variant="secondary" isDisabled={busy} tooltip="Reboot the VM; nested workspaces come back with it" onPress={() => run("restart")}>Restart VM</Button>
                <Button size="sm" variant="secondary" isDisabled={busy} tooltip="Shut down the VM and every workspace inside it" onPress={() => setConfirm("stop")}>Stop VM</Button>
              </>
            ) : (
              <Button size="sm" variant="secondary" isDisabled={busy || t.status !== "active"} tooltip="Boot the VM; workspaces that were running start again" onPress={() => run("start")}>Start VM</Button>
            )}
          </div>
        </div>
      )}
      <ConfirmDialog
        isOpen={confirm === "stop"}
        onOpenChange={(o) => !o && setConfirm(undefined)}
        title={`Stop the VM of ${t.name}?`}
        body={`All ${v?.containers?.length ?? 0} workspace(s) inside it stop with it. Legacy direct workspaces are not affected.`}
        confirmLabel="Stop VM"
        onConfirm={() => run("stop")}
      />
      <ConfirmDialog
        isOpen={confirm === "enable"}
        onOpenChange={(o) => !o && setConfirm(undefined)}
        title={`Create a tenant VM for ${t.name}?`}
        body="A KVM VM sized from the tenant's plan is created on its encrypted pool. New workspaces go into it. Existing workspaces are not moved or restarted."
        confirmLabel="Create VM"
        onConfirm={() => run("enable")}
      />
    </section>
  );
}
