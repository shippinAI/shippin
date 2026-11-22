import { Database } from "lucide-react";
import type { PageMeta } from "../../../../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "tenant.database",
      section: "devspace",
      order: 30,
      label: "Database",
      icon: Database,
      hue: "var(--color-hue-data)",
      mobile: true,
      feature: "workspaces",
    },
  ],
} satisfies PageMeta;
