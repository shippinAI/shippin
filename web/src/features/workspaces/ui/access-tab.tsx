import { KeyRound } from "lucide-react";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { LinkButton } from "../../../lib/design/components/button/link-button";
import { CodeBlock } from "../../../lib/design/components/code-block/code-block";
import type { Workspace } from "../../../lib/api/types";

/** Optional connection info. Nothing here is required: Open works without it. */
export function AccessTab({ ws }: { ws: Workspace }) {
  const c = ws.connect;
  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1.4fr_1fr]">
      <Card>
        <CardHeader title="Connection info" description="Optional. Use these if you prefer a local terminal or editor." />
        <div className="space-y-4 p-5">
          {c ? (
            <>
              <CodeBlock label="SSH" code={c.ssh} />
              <CodeBlock label="~/.ssh/config" code={c.ssh_config} />
            </>
          ) : (
            <p className="text-sm text-fg-muted">Available once the workspace is running.</p>
          )}
        </div>
      </Card>
      <Card>
        <CardHeader title="SSH keys" description={`${ws.ssh_key_count} key${ws.ssh_key_count === 1 ? "" : "s"} installed`} />
        <div className="space-y-4 p-5 text-sm text-fg-muted">
          <p>Keys come from your account and are added to every workspace you own, including running ones.</p>
          <LinkButton href="/account/ssh-keys" variant="secondary" size="sm" tooltip="Add or remove SSH keys">
            <KeyRound className="size-4" /> Manage SSH keys
          </LinkButton>
        </div>
      </Card>
    </div>
  );
}
