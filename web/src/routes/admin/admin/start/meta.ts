import { Rocket } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    { id: "admin.get-started", section: "admin.start", order: 10, label: "Get started", icon: Rocket, hue: "var(--color-primary)", cap: "platform.settings.read" },
  ],
} satisfies PageMeta;
