import { Box } from "lucide-react";
import type { VMContainer } from "../../../../lib/api/vm-types";
import { formatBytes } from "../../../../lib/format";
import { StatusBadge } from "../status-badge";

/** Workspace containers nested in the tenant VM (names, state, inner IP, memory). */
export function VMContainers({ items }: { items: VMContainer[] }) {
  if (items.length === 0) {
    return <p className="text-xs text-fg-subtle">No nested workspaces yet. New workspaces of this tenant are created inside the VM.</p>;
  }
  return (
    <ul className="divide-y divide-line rounded-lg border border-line" aria-label="Nested workspace containers">
      {items.map((c) => (
        <li key={c.name} className="flex items-center gap-3 px-3 py-2 text-sm">
          <Box className="size-3.5 shrink-0 text-fg-subtle" strokeWidth={1.75} />
          <span className="min-w-0 flex-1 truncate font-mono text-fg" title={c.name}>{c.name}</span>
          <span className="font-mono text-fg-muted tabular-nums">{c.ipv4 || "—"}</span>
          <span className="w-16 text-right text-fg-muted tabular-nums">{formatBytes(c.memory_bytes)}</span>
          <StatusBadge status={c.status} />
        </li>
      ))}
    </ul>
  );
}
