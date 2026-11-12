import type { ReactNode } from "react";
import { Button } from "../../../lib/design/components/button/button";

/** Recovery codes, shown the one time they exist. `next` replaces the default "continue" action. */
export function RecoveryCodes({ codes, next }: { codes: string[]; next?: ReactNode }) {
  return (
    <div>
      <h1 className="text-3xl font-semibold tracking-tight">Save your recovery codes</h1>
      <p className="mt-2 text-base text-fg-muted">
        They are shown only this once. Each one signs you in a single time from the sign-in page; keep them somewhere safe.
      </p>
      <ul className="mt-6 grid grid-cols-2 gap-2 rounded-xl border border-line bg-surface p-5 font-mono text-base">
        {codes.map((c) => (
          <li key={c}>{c}</li>
        ))}
      </ul>
      <div className="mt-4 flex gap-3">
        <Button variant="secondary" tooltip="Copy all recovery codes" onPress={() => void navigator.clipboard?.writeText(codes.join("\n"))}>Copy</Button>
        {next ?? <Button variant="primary" tooltip="Open the panel" onPress={() => window.location.assign("/")}>I saved them, continue</Button>}
      </div>
    </div>
  );
}
