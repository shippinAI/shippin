import { Server } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "admin.host",
      section: "admin.infrastructure",
      order: 20,
      label: "Host",
      icon: Server,
      hue: "var(--color-hue-data)",
      cap: "host.read",
      feature: "workspaces",
    },
  ],
} satisfies PageMeta;
