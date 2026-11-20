import { SquareTerminal } from "lucide-react";
import type { PageMeta } from "../../../../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "tenant.devspaces",
      section: "devspace",
      order: 60,
      label: "Devspaces",
      icon: SquareTerminal,
      hue: "var(--color-hue-workspaces)",
      feature: "workspaces",
    },
  ],
} satisfies PageMeta;
