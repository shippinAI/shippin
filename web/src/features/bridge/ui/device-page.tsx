import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { apiGet, apiSend, ApiError } from "../../../lib/api/client";
import { useMe } from "../../../lib/hooks/use-api";
import { useIdpName } from "../../../lib/hooks/use-idp-name";
import { useSearchParams } from "../../../lib/router/router";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { TextField } from "../../../lib/design/components/field/text-field";
import { BrandMark } from "../../../template/brand-mark";
import { deviceSignInURL } from "../device-sign-in";

/** Legacy fallback for sign-ins started before the return URL was passed to OIDC. */
export const pendingDeviceKey = "shippin.bridge.device";

/** Confirms a Shippin bridge sign-in (device code) for the signed-in account. */
export function DevicePage() {
  const [params] = useSearchParams();
  const [code, setCode] = useState(params.get("code") ?? sessionStorage.getItem(pendingDeviceKey) ?? "");
  const me = useMe();
  const idp = useIdpName();
  useEffect(() => {
    if (me.isSuccess) sessionStorage.removeItem(pendingDeviceKey);
  }, [me.isSuccess]);
  const decide = useMutation({
    mutationFn: (approve: boolean) => apiSend<{ approved: boolean }>("POST", `/v1/bridge/device/${encodeURIComponent(code)}`, { approve }),
  });
  const pending = useQuery({
    queryKey: ["bridge-device", code],
    queryFn: () => apiGet<{ client_name: string }>(`/v1/bridge/device/${encodeURIComponent(code)}`),
    enabled: me.isSuccess && !decide.data && code.replace(/[^A-Za-z0-9]/g, "").length === 8,
    retry: false,
    refetchInterval: (query) => query.state.error ? false : 5_000,
  });
  const signedOut = me.error instanceof ApiError && me.error.status === 401;
  const canDecide = !!pending.data && !pending.isError && !decide.isPending;
  const email = me.data?.email;
  return (
    <main className="flex min-h-full items-center justify-center p-6">
      <div className="w-full max-w-md space-y-4">
        <BrandMark size={32} />
        <Card>
          <CardHeader title="Connect the Shippin bridge" description="A device wants to open your devspaces with your account." />
          <div className="space-y-4 p-4">
            {signedOut && (
              <>
                <p className="text-sm">Sign in with {idp} first; this code is kept for when you come back.</p>
                <Button variant="primary" tooltip="Sign in, then come back to confirm the code" onPress={() => window.location.assign(deviceSignInURL(code))}>Continue with {idp}</Button>
              </>
            )}
            {me.isPending && <p role="status" className="text-sm">Checking your sign-in…</p>}
            {me.isError && !signedOut && <p role="alert" className="text-sm text-danger">Unable to check your sign-in. Reload this page to try again.</p>}
            {me.isSuccess && decide.data == null && (
              <>
                <TextField label="Code shown by the bridge" value={code} onChange={(next) => { setCode(next); decide.reset(); }} mono placeholder="XXXX-XXXX" />
                {pending.data && !pending.isError && (
                  <p className="text-sm">
                    <strong>{pending.data.client_name}</strong> will act as <strong>{email}</strong> on the workspaces you can open,
                    with certificates that expire within minutes. You can revoke it any time.
                  </p>
                )}
                {pending.error instanceof ApiError && <p role="alert" className="text-sm text-danger">{pending.error.message}</p>}
                <div className="flex gap-2">
                  <Button variant="primary" tooltip="Sign this bridge in as you" isDisabled={!canDecide} onPress={() => decide.mutate(true)}>Allow</Button>
                  <Button variant="secondary" tooltip="Refuse this sign-in" isDisabled={!canDecide} onPress={() => decide.mutate(false)}>Deny</Button>
                </div>
              </>
            )}
            {decide.data && (
              <p role="status" className="text-sm">
                {decide.data.approved ? "Approved. The waiting bridge will connect automatically. You can return to your terminal." : "Denied. The bridge was not signed in."}
              </p>
            )}
            {decide.error instanceof ApiError && <p role="alert" className="text-sm text-danger">{decide.error.message}</p>}
          </div>
        </Card>
      </div>
    </main>
  );
}
