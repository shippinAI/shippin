import { ExternalLink } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import type { Workspace } from "../../../lib/api/types";

/** Primary action: open the browser IDE over HTTPS in a new tab. */
export function OpenButton({ ws, size = "sm" }: { ws: Workspace; size?: "sm" | "md" }) {
  const ready = ws.state === "running";
  return (
    <Button
      variant="primary"
      size={size}
      isDisabled={!ready}
      tooltip={ready ? `Open the browser IDE at ${ws.host}` : "Power the workspace on to open it"}
      onPress={() => window.open(ws.url, "_blank", "noopener")}
    >
      <ExternalLink className="size-4" /> Open
    </Button>
  );
}
