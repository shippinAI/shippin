import { SlidersHorizontal } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    { id: "admin.platform", section: "admin.platform", order: 40, label: "Platform settings", icon: SlidersHorizontal, hue: "var(--color-hue-data)", cap: "platform.settings.read" },
  ],
} satisfies PageMeta;
