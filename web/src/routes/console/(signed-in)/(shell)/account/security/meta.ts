import { KeyRound } from "lucide-react";
import type { PageMeta } from "../../../../../../lib/router/meta";

export default {
  nav: [
    { id: "account.security", section: "account", order: 20, label: "Security", icon: KeyRound, hue: "var(--color-hue-secrets)", cap: "account.local" },
  ],
} satisfies PageMeta;
