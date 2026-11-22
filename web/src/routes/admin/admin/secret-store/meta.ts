import { KeyRound } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "admin.secret-store",
      section: "admin.platform",
      order: 30,
      label: "Secret store",
      icon: KeyRound,
      hue: "var(--color-hue-secrets)",
      cap: "platform.settings.read",
    },
  ],
} satisfies PageMeta;
