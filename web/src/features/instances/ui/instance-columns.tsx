import { Container, Monitor } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import type { ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import type { Instance } from "../../../lib/api/types";
import { formatBytes, timeAgo } from "../../../lib/format";

export type InstanceRow = Instance & { id: string };

export const instanceColumns: ColumnDef<InstanceRow>[] = [
  {
    id: "name",
    label: "Name",
    isRowHeader: true,
    render: (i) => (
      <span className="flex items-center gap-2.5">
        <span className="flex size-7 items-center justify-center rounded-md bg-raised ring-1 ring-line">
          {i.type === "container" ? <Container className="size-3.5 text-hue-edge" /> : <Monitor className="size-3.5 text-hue-identity" />}
        </span>
        <span className="font-medium text-fg">{i.name}</span>
      </span>
    ),
  },
  {
    id: "status",
    label: "Status",
    render: (i) => (
      <Badge tone={i.status === "Running" ? "success" : i.status === "Stopped" ? "neutral" : "warning"}>
        <StatusDot health={i.status === "Running" ? "up" : "unknown"} /> {i.status}
      </Badge>
    ),
  },
  { id: "ipv4", label: "IPv4", render: (i) => <span className="font-mono text-[12.5px]">{i.ipv4 || "—"}</span> },
  { id: "image", label: "Image", render: (i) => <span className="capitalize">{i.image.trim() || "—"}</span> },
  { id: "mem", label: "Memory", render: (i) => <span className="tabular-nums">{formatBytes(i.memory_bytes)}</span> },
  { id: "procs", label: "Procs", render: (i) => <span className="tabular-nums">{i.processes || "—"}</span> },
  {
    id: "boot",
    label: "Autostart",
    render: (i) => <Badge tone={i.autostart ? "primary" : "neutral"}>{i.autostart ? "On" : "Off"}</Badge>,
  },
  { id: "created", label: "Created", render: (i) => timeAgo(i.created_at) },
];
