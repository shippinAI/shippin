import { Users } from "lucide-react";
import type { PageMeta } from "../../../../../../../lib/router/meta";

export default {
  nav: [{ id: "tenant.team", section: "devspace", order: 170, label: "Team", icon: Users, hue: "var(--color-hue-billing)" }],
} satisfies PageMeta;
