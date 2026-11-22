import { Cpu } from "lucide-react";
import type { PageMeta } from "../../../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "infra.hosts",
      section: "infrastructure",
      order: 10,
      label: "Hosts & nodes",
      icon: Cpu,
      hue: "var(--color-hue-edge)",
      cap: "infra.hosts",
      prefix: true,
      mobile: true,
    },
  ],
} satisfies PageMeta;
