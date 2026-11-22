import { Layers } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "admin.plans",
      section: "admin.customers",
      order: 40,
      label: "Plans & tiers",
      icon: Layers,
      hue: "var(--color-hue-ai)",
      cap: "catalog.plans.read",
      feature: "catalog",
    },
  ],
} satisfies PageMeta;
