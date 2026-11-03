import { Check, Copy, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import type { EnrollToken } from "../../../lib/api/server-types";

type Props = { token: EnrollToken; onDone: () => void };

/** Shows a single-use token once, with a copy button. Closing forgets it. */
export function TokenReveal({ token, onDone }: Props) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(token.token);
      setCopied(true);
      toast.success("Token copied", "Paste it into hostd's enrollment token file.");
    } catch {
      toast.error("Copy failed", "Your browser blocked clipboard access. Select the token and copy it.");
    }
  };
  const until = new Date(token.expires_at).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  return (
    <div className="space-y-4 px-6 py-5">
      <div className="flex gap-2 rounded-[var(--radius-control)] border border-warning/30 bg-warning/10 p-3 text-[13px] text-fg">
        <TriangleAlert className="mt-0.5 size-4 shrink-0 text-warning" aria-hidden />
        <p>This token is shown <strong>once</strong>. It works one time, until {until}. Shippin keeps only a hash of it.</p>
      </div>
      <div className="flex items-center gap-2 rounded-[var(--radius-control)] border border-line bg-canvas/60 py-1 pr-1 pl-3">
        <code aria-label="Enrollment token" className="min-w-0 flex-1 truncate font-mono text-[12.5px] text-fg select-all">{token.token}</code>
        <Button variant="secondary" size="sm" aria-label="Copy token" tooltip={copied ? "Copied" : "Copy the token to the clipboard"} onPress={copy}>
          {copied ? <Check className="size-4" aria-hidden /> : <Copy className="size-4" aria-hidden />}{copied ? "Copied" : "Copy"}
        </Button>
      </div>
      <ol className="list-decimal space-y-1 pl-5 text-[13px] text-fg-muted">
        <li>On the server, write the token to the file named by <code className="font-mono">enroll.token_file</code> in hostd.json.</li>
        <li>Restart hostd. It enrolls, deletes the token file and starts sending heartbeats.</li>
        <li>This page turns the server <em>Active</em> on the first heartbeat.</li>
      </ol>
      <div className="flex justify-end border-t border-line pt-4">
        <Button variant="primary" tooltip="Forget the token and close" onPress={onDone}>Done</Button>
      </div>
    </div>
  );
}
