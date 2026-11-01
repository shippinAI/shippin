import { useState } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { useAdminWorkspaces } from "../../../../lib/hooks/use-admin";
import { BreakGlassDialog } from "./break-glass-dialog";
import { RequestAccessDialog } from "./request-access-dialog";
import type { Target } from "./scope-select";

type Mode = "request" | "breakglass";

/** Builds the dialog target (tenant + its workspaces) and renders the chosen dialog. */
export function AccessDialogs({ tenantId, tenantName, workspaceId, mode, onClose }: { tenantId: string; tenantName: string; workspaceId?: string; mode?: Mode; onClose: () => void }) {
  const wss = useAdminWorkspaces().data?.items ?? [];
  const target: Target | undefined = mode
    ? { tenantId, tenantName, workspaceId, workspaces: wss.filter((w) => w.tenant_id === tenantId).map((w) => ({ id: w.id, name: w.name })) }
    : undefined;
  return (
    <>
      <RequestAccessDialog target={mode === "request" ? target : undefined} onClose={onClose} />
      <BreakGlassDialog target={mode === "breakglass" ? target : undefined} onClose={onClose} />
    </>
  );
}

/** Tenant drawer buttons: ask the customer for access, or break glass in an emergency. */
export function AccessLauncher({ tenantId, tenantName, disabled }: { tenantId: string; tenantName: string; disabled?: boolean }) {
  const [mode, setMode] = useState<Mode>();
  return (
    <>
      <Button size="sm" variant="primary" isDisabled={disabled} tooltip="Ask the customer's owners/admins for time-boxed access" onPress={() => setMode("request")}>
        Request access
      </Button>
      <Button size="sm" variant="danger" isDisabled={disabled} tooltip="Emergency access without approval (audited, customer notified)" onPress={() => setMode("breakglass")}>
        Break glass
      </Button>
      <AccessDialogs tenantId={tenantId} tenantName={tenantName} mode={mode} onClose={() => setMode(undefined)} />
    </>
  );
}
