import { Fingerprint } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "admin.identity",
      section: "admin.people",
      order: 20,
      label: "Users & sign-in",
      icon: Fingerprint,
      hue: "var(--color-hue-identity)",
      cap: "identity.read",
      feature: "idp_admin",
    },
  ],
} satisfies PageMeta;
