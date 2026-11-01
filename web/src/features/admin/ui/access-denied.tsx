import { ShieldAlert } from "lucide-react";
import { LinkButton } from "../../../lib/design/components/button/link-button";

/** Shown when /v1/operator/access returns 403 (or the session is gone). */
export function AccessDenied({ status, message }: { status: number; message: string }) {
  return (
    <div className="flex h-full items-center justify-center p-8">
      <div className="max-w-md rounded-2xl border border-line bg-surface p-8 text-center shadow-[var(--shadow-e2)]">
        <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-full bg-danger/12 text-danger">
          <ShieldAlert className="size-6" strokeWidth={1.75} />
        </div>
        <div className="text-xs font-medium tracking-wider text-fg-subtle uppercase">{status || "Error"}</div>
        <h1 className="mt-1 text-xl font-semibold tracking-tight">Provider panel unavailable</h1>
        <p className="mt-2 text-sm text-fg-muted">{message}</p>
        <div className="mt-6 flex justify-center">
          <LinkButton href="/" tooltip="Return to your organization" variant="secondary">Back to console</LinkButton>
        </div>
      </div>
    </div>
  );
}
