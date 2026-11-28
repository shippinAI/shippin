import { Building2 } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "admin.tenants",
      section: "admin.customers",
      order: 10,
      label: "Tenants",
      icon: Building2,
      hue: "var(--color-primary)",
      cap: "tenants.read",
      feature: "catalog",
    },
  ],
} satisfies PageMeta;
