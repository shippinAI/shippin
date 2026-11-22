import { ShieldCheck } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "admin.access",
      section: "admin.people",
      order: 30,
      label: "Access & audit",
      icon: ShieldCheck,
      hue: "var(--color-danger)",
      cap: "audit.read|access.request|access.approve_breakglass",
    },
  ],
} satisfies PageMeta;
