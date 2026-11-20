import { Settings } from "lucide-react";
import type { PageMeta } from "../../../../../../../lib/router/meta";

export default {
  nav: [{ id: "tenant.settings", section: "devspace", order: 180, label: "Settings", icon: Settings, hue: "var(--color-fg-muted)" }],
} satisfies PageMeta;
