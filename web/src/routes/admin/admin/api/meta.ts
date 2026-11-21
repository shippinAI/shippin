import { Braces } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [{ id: "admin.api", section: "admin.developer", order: 10, label: "API explorer", icon: Braces, hue: "var(--color-hue-data)" }],
} satisfies PageMeta;
