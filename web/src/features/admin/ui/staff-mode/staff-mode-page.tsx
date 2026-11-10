import { ShieldCheck } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Navigate, useNavigate, useSearchParams } from "../../../../lib/router/router";
import { useEnterStaffMode, useStaffMode } from "../../../../lib/hooks/use-staff-mode";

/** Entry point of the tenant → provider panel switch: one reason, audited, then the panel. */
export function StaffModePage() {
  const [params] = useSearchParams();
  const from = params.get("from") ?? "";
  const rd = params.get("rd") ?? "/admin/tenants";
  const target = rd.startsWith("/admin/") || rd === "/operator" || rd.startsWith("/operator/") ? rd : "/admin/tenants";
  const mode = useStaffMode().data;
  const enter = useEnterStaffMode();
  const nav = useNavigate();
  const [reason, setReason] = useState("");
  const len = reason.trim().length;
  const valid = len >= 8 && len <= 300;
  if (mode?.active) return <Navigate to={target} replace />;
  return (
    <div className="mx-auto max-w-lg">
      <Card>
        <CardHeader title="Enter staff mode" description={`You're switching into the provider panel for ${mode?.minutes ?? 60} minutes. You may be asked to sign in again; the reason and the tenant you came from go in the audit log.`} />
        <div className="space-y-4 px-4 pb-4">
          <TextField label="Reason" multiline autoFocus value={reason} onChange={setReason} placeholder="e.g. Support ticket 1234: tenant can't start workspaces" description="8 to 300 characters." />
          {enter.isError && <p role="alert" className="text-base text-danger">{enter.error.message}</p>}
          <div className="flex justify-end">
            <Button variant="primary" isDisabled={!valid || enter.isPending} tooltip={valid ? "Record the reason and open the panel" : "Give a reason first"}
              onPress={() => enter.mutate({ reason: reason.trim(), from_tenant: from }, { onSuccess: () => nav(target) })}>
              <ShieldCheck className="size-4" /> Enter staff mode
            </Button>
          </div>
        </div>
      </Card>
    </div>
  );
}
