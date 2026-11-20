import { UserPlus } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "admin.signups",
      section: "admin.customers",
      order: 30,
      label: "Sign-ups",
      icon: UserPlus,
      hue: "var(--color-primary)",
      cap: "customers.read|acquisition.read|platform.settings.write",
    },
  ],
} satisfies PageMeta;
