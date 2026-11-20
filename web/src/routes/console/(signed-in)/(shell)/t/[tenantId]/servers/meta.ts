import { Server } from "lucide-react";
import type { PageMeta } from "../../../../../../../lib/router/meta";

export default {
  nav: [
    { id: "tenant.servers", section: "devspace", order: 130, label: "Servers", icon: Server, hue: "var(--color-hue-edge)", feature: "servers" },
  ],
} satisfies PageMeta;
