import { Globe } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    { id: "admin.edge", section: "admin.infrastructure", order: 30, label: "Edge", icon: Globe, hue: "var(--color-hue-edge)", cap: "edge.read", feature: "workspaces" },
  ],
} satisfies PageMeta;
