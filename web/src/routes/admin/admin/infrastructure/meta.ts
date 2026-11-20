import { Cpu } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    { id: "admin.infrastructure", section: "admin.infrastructure", order: 10, label: "Hosts & guests", icon: Cpu, hue: "var(--color-hue-edge)", cap: "infra.read", prefix: true },
  ],
} satisfies PageMeta;
