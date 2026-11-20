import { LayoutGrid } from "lucide-react";
import type { PageMeta } from "../../../../../lib/router/meta";

export default {
  nav: [
    { id: "account.hub", section: "account", order: 10, label: "Approvals", icon: LayoutGrid, hue: "var(--color-hue-ai)", feature: "board" },
  ],
} satisfies PageMeta;
