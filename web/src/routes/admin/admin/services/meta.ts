import { Blocks } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "admin.services",
      section: "admin.customers",
      order: 50,
      label: "Service catalog",
      icon: Blocks,
      hue: "var(--color-hue-data)",
      cap: "catalog.plans.read",
      feature: "catalog",
    },
  ],
} satisfies PageMeta;
