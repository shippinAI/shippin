import { LayoutGrid } from "lucide-react";
import type { PageMeta } from "../../../../../../lib/router/meta";

export default {
  nav: [{ id: "tenant.home", section: "devspace", order: 10, label: "Home", icon: LayoutGrid, hue: "var(--color-primary)", mobile: true }],
} satisfies PageMeta;
