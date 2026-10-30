import { useEffect, useState } from "react";
import { loadNodes, type ComputeNode } from "./inventory";

type Inventory = { state: "loading" } | { state: "error"; message: string } | { state: "ready"; nodes: ComputeNode[] };

export function InventoryPanel({ apiBaseUrl, token }: { apiBaseUrl: string; token: string }) {
  const [inventory, setInventory] = useState<Inventory>({ state: "loading" });
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setInventory({ state: "loading" });
    loadNodes(apiBaseUrl, token, controller.signal).then(
      (nodes) => { if (!controller.signal.aborted) setInventory({ state: "ready", nodes }); },
      (error: unknown) => {
        if (!controller.signal.aborted) setInventory({ state: "error", message: error instanceof Error ? error.message : "Infrastructure inventory is unavailable." });
      }
    );
    return () => controller.abort();
  }, [apiBaseUrl, token, revision]);

  return <section className="panel" aria-labelledby="infrastructure-heading">
    <h3 id="infrastructure-heading">Infrastructure</h3>
    <p className="muted">Read-only node inventory. Node enrollment and host management are not implemented yet.</p>
    <button type="button" onClick={() => setRevision((value) => value + 1)} disabled={inventory.state === "loading"}>Refresh inventory</button>
    {inventory.state === "loading" ? <p role="status">Loading infrastructure?</p> : null}
    {inventory.state === "error" ? <p className="error-banner" role="alert">{inventory.message} Check the control plane connection and your access; the inventory service is not implemented in this repository.</p> : null}
    {inventory.state === "ready" && !inventory.nodes.length ? <p>No infrastructure nodes returned by the control plane.</p> : null}
    {inventory.state === "ready" && inventory.nodes.length ? <div className="inventory-list">{inventory.nodes.map((node) => <article className="inventory-node" key={node.id}>
      <h4>{node.name}</h4>
      <p>Provider: {node.provider} ? Status: {node.status} ? Schedulable: {node.schedulable ? "Yes" : "No"}</p>
      <p className="muted">ID: {node.id}{node.last_seen_at ? ` ? Last seen: ${node.last_seen_at}` : ""}</p>
      <p>Verified capabilities: {node.capabilities.length ? node.capabilities.join(", ") : "None reported"}</p>
      {node.readiness_reasons?.length ? <p>Readiness: {node.readiness_reasons.join("; ")}</p> : null}
    </article>)}</div> : null}
  </section>;
}
