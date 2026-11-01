import { useState } from "react";
import { Button } from "../../lib/design/components/button/button";
import { LinkButton } from "../../lib/design/components/button/link-button";
import type { SparkEndpoint as Endpoint } from "./use-sparks";

export function SparkEndpoint({ endpoint, modelAPI = false }: { endpoint: Endpoint; modelAPI?: boolean }) {
  const [copied, setCopied] = useState("");
  const published = endpoint.state === "published" && !!endpoint.url;
  async function copy() {
    try {
      await navigator.clipboard.writeText(endpoint.url!);
      setCopied("API URL copied");
    } catch {
      setCopied("Select and copy the URL above.");
    }
  }
  return (
    <div className="border-t border-line pt-3">
      <div className="flex items-center justify-between gap-3 text-sm">
        <span className="font-medium">{endpoint.name}</span>
        <span className="text-xs text-fg-subtle">{published ? "Published" : "Not connected"}</span>
      </div>
      {!published && <p className="mt-2 text-xs text-fg-subtle">{endpoint.reason}</p>}
      {published && (
        <div className="mt-3 space-y-3">
          <code className="block break-all text-xs">{endpoint.url}</code>
          {modelAPI ? (
            <>
              <Button size="sm" tooltip="Copy the HTTPS base URL for your API client" onPress={copy}>Copy API URL</Button>
              <p className="text-xs text-fg-subtle">API clients also need their own authorized model-gateway token.</p>
              <span role="status" className="text-xs text-fg-subtle">{copied}</span>
            </>
          ) : <LinkButton href={endpoint.url!} tooltip={`Open ${endpoint.name} using your Nomen sign-in`} size="sm">Open {endpoint.name}</LinkButton>}
        </div>
      )}
    </div>
  );
}
