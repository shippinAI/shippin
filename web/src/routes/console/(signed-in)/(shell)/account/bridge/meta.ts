import { Cable } from "lucide-react";
import type { PageMeta } from "../../../../../../lib/router/meta";

export default {
  nav: [
    { id: "account.bridge", section: "account", order: 70, label: "Bridge sessions", icon: Cable, hue: "var(--color-hue-secrets)", feature: "bridge" },
  ],
} satisfies PageMeta;
