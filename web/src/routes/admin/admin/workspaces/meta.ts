import { Boxes } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    { id: "admin.workspaces", section: "admin.customers", order: 70, label: "Workspaces", icon: Boxes, hue: "var(--color-hue-workspaces)", cap: "workspaces.read", feature: "workspaces" },
  ],
} satisfies PageMeta;
