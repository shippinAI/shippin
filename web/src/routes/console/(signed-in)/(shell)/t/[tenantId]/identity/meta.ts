import { Fingerprint } from "lucide-react";
import type { PageMeta } from "../../../../../../../lib/router/meta";

export default {
  nav: [
    { id: "tenant.identity", section: "devspace", order: 140, label: "Identity", icon: Fingerprint, hue: "var(--color-hue-identity)" },
  ],
} satisfies PageMeta;
