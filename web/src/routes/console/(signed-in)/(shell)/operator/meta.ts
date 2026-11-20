import { Gauge } from "lucide-react";
import type { PageMeta } from "../../../../../lib/router/meta";

export default {
  nav: [
    { id: "operator.health", section: "operator", order: 10, label: "Platform health", icon: Gauge, hue: "var(--color-hue-data)" },
  ],
} satisfies PageMeta;
