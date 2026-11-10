import { AlertTriangle, Loader2, RotateCw } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import type { TenantRef } from "../../../lib/api/types";
import { useRetryProvision } from "../../../lib/hooks/use-provision";

/** Shown while a tenant isn't active: provisioning in progress, or failed with a Retry. */
export function SetupBanner({ tenant }: { tenant: TenantRef }) {
  const retry = useRetryProvision();
  if (tenant.status === "active") return null;
  const failed = tenant.status === "failed";
  return (
    <div className={`mb-8 flex items-center gap-4 rounded-xl border px-5 py-4 ${failed ? "border-danger/30 bg-danger/10" : "border-info/30 bg-info/10"}`}>
      {failed ? <AlertTriangle className="size-5 shrink-0 text-danger" /> : <Loader2 className="size-5 shrink-0 animate-spin text-info" />}
      <div className="flex-1">
        <div className="text-base font-medium text-fg">{failed ? "Setup didn't finish" : "Setting up your organization"}</div>
        <div className="text-sm text-fg-muted">
          {failed ? "Your organization exists, but its resources aren't ready yet. Retrying is safe." : "Your network and storage are being created."}
        </div>
      </div>
      {failed && (
        <Button
          variant="primary"
          size="sm"
          isDisabled={!tenant.can_manage || retry.isPending}
          tooltip={tenant.can_manage ? "Run setup again" : "Ask an owner or admin to retry"}
          onPress={() => retry.mutate({ id: tenant.tenant_id, name: tenant.name })}
        >
          {retry.isPending ? <Loader2 className="size-4 animate-spin" /> : <RotateCw className="size-4" />} Retry setup
        </Button>
      )}
    </div>
  );
}
