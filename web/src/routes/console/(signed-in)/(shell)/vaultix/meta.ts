import { KeyRound } from "lucide-react";
import type { PageMeta } from "../../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "tenant.secrets",
      section: "devspace",
      order: 120,
      label: "Secrets",
      icon: KeyRound,
      hue: "var(--color-hue-secrets)",
      feature: "secrets",
    },
  ],
} satisfies PageMeta;
