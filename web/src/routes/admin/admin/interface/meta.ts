import { Palette } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    { id: "admin.interface", section: "admin.platform", order: 50, label: "Interface", icon: Palette, hue: "var(--color-hue-data)", cap: "platform.settings.read" },
  ],
} satisfies PageMeta;
