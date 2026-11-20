import { KeySquare } from "lucide-react";
import type { PageMeta } from "../../../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "account.ssh",
      section: "account",
      order: 40,
      label: "SSH keys",
      icon: KeySquare,
      hue: "var(--color-hue-secrets)",
      feature: "workspaces",
    },
  ],
} satisfies PageMeta;
