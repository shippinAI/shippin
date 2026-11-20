import { Database } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    { id: "admin.built-in", section: "admin.platform", order: 10, label: "Built-in services", icon: Database, hue: "var(--color-hue-data)", cap: "platform.settings.read" },
  ],
} satisfies PageMeta;
