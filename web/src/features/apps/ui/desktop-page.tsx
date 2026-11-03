import { useEffect, useRef, useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { useParams } from "../../../lib/router/router";
import { PageHeader } from "../../../template/page-header";
import { AppCards } from "./app-cards";

/** Browser access to the tenant VM desktop. The session stays when this tab closes. */
export function DesktopPage() {
  const { tenantId = "" } = useParams();
  return (
    <>
      <PageHeader
        eyebrow="Virtual machine"
        title="Desktop"
        description="Open this org's VM desktop in the browser. You sign in at the VM's own login screen. Closing the tab disconnects the viewer and leaves the desktop session running, which is what Claude Dispatch needs."
      />
      <DesktopPipe tenantId={tenantId} />
      <div className="mt-8">
        <h2 className="mb-3 text-sm font-semibold text-fg">Install apps yourself</h2>
        <p className="mb-4 max-w-2xl text-sm text-fg-muted">
          Paste a command in your own terminal on the VM. It installs the app and starts the vendor's sign-in, so the subscription is yours. Shippin does not store vendor credentials.
        </p>
        <AppCards tenantId={tenantId} />
      </div>
    </>
  );
}

function DesktopPipe({ tenantId }: { tenantId: string }) {
  const [state, setState] = useState<"idle" | "open" | "closed">("idle");
  const sock = useRef<WebSocket | null>(null);
  useEffect(() => () => sock.current?.close(), []);
  const connect = () => {
    sock.current?.close();
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${proto}//${location.host}/v1/tenants/${tenantId}/desktop`);
    ws.binaryType = "arraybuffer";
    ws.onopen = () => setState("open");
    ws.onclose = () => setState("closed");
    sock.current = ws;
  };
  return (
    <section className="rounded-[var(--radius-card)] border border-line bg-surface p-5">
      <p className="max-w-2xl text-sm text-fg-muted">
        The console opens an authenticated pipe to the desktop service on the VM's loopback address. Nothing is published on a new port. This page does not draw the desktop picture yet: a browser client that can speak the VM's existing listener, without turning off its TLS, is still an open choice. Connecting keeps the pipe up; closing the tab does not log the desktop out.
      </p>
      <div className="mt-4 flex items-center gap-3">
        <Button size="sm" variant="primary" tooltip="Open the authenticated pipe to the VM desktop" onPress={connect}>
          Connect desktop
        </Button>
        <span className="text-xs text-fg-muted">
          {state === "idle" && "Not connected"}
          {state === "open" && "Pipe open. Closing this tab leaves the VM session logged in."}
          {state === "closed" && "Pipe closed. The VM session stays logged in."}
        </span>
      </div>
    </section>
  );
}
