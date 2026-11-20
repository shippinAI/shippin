import { ShieldCheck } from "lucide-react";
import type { PageMeta } from "../../../../../lib/router/meta";

export default {
  nav: [
    { id: "operator.panel", section: "operator", order: 30, label: "Provider panel", icon: ShieldCheck, to: "/admin", hue: "var(--color-warning)", cap: "provider.panel" },
  ],
} satisfies PageMeta;
