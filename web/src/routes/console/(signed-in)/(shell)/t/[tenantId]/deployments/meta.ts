import { Package } from "lucide-react";
import type { PageMeta } from "../../../../../../../lib/router/meta";

export default {
  nav: [
    { id: "tenant.deployments", section: "devspace", order: 40, label: "Services", icon: Package, hue: "var(--color-hue-data)", mobile: true, feature: "servers" },
  ],
} satisfies PageMeta;
