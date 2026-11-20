import { Network } from "lucide-react";
import type { PageMeta } from "../../../../../../../lib/router/meta";

export default {
  nav: [
    { id: "tenant.network", section: "devspace", order: 150, label: "Network", icon: Network, hue: "var(--color-hue-network)", soon: "Per-org network policy lands with hypervisor pools" },
  ],
} satisfies PageMeta;
