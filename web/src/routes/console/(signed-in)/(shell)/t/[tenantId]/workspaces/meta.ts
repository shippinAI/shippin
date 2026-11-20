import { Boxes } from "lucide-react";
import type { PageMeta } from "../../../../../../../lib/router/meta";

export default {
  nav: [
    { id: "tenant.workspaces", section: "devspace", order: 50, label: "Workspaces", icon: Boxes, hue: "var(--color-hue-workspaces)", mobile: true, feature: "workspaces" },
  ],
} satisfies PageMeta;
