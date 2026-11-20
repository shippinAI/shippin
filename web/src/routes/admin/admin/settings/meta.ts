import { SlidersHorizontal } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    { id: "admin.settings", section: "admin.platform", order: 20, label: "Settings", icon: SlidersHorizontal, hue: "var(--color-hue-data)", cap: "platform.settings.read", prefix: true },
  ],
} satisfies PageMeta;
