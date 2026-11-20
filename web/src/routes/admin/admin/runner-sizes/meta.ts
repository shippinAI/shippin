import { Cpu } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    { id: "admin.runner-sizes", section: "admin.customers", order: 60, label: "Runner sizes", icon: Cpu, hue: "var(--color-hue-data)", cap: "catalog.plans.read", feature: "catalog" },
  ],
} satisfies PageMeta;
