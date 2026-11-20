import { Server } from "lucide-react";
import type { PageMeta } from "../../../../../../lib/router/meta";

export default {
  nav: [
    { id: "operator.instances", section: "operator", order: 20, label: "Instances", icon: Server, hue: "var(--color-hue-edge)", unlessCap: "infra.hosts" },
  ],
} satisfies PageMeta;
