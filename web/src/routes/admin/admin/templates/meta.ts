import { LayoutTemplate } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "admin.templates",
      section: "admin.workloads",
      order: 80,
      label: "Templates",
      icon: LayoutTemplate,
      hue: "var(--color-hue-ai)",
      cap: "templates.read",
      feature: "workspaces",
    },
  ],
} satisfies PageMeta;
