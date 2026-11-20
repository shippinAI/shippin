import { Cpu } from "lucide-react";
import type { PageMeta } from "../../../../../../../lib/router/meta";

export default {
  nav: [
    { id: "tenant.runners", section: "devspace", order: 100, label: "Build runners", icon: Cpu, hue: "var(--color-hue-workspaces)", feature: "workspaces", prefix: true },
  ],
} satisfies PageMeta;
