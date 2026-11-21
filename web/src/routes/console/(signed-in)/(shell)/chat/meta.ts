import { Bot } from "lucide-react";
import type { PageMeta } from "../../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "account.chat",
      section: "account",
      order: 50,
      label: "Automaton chat",
      icon: Bot,
      hue: "var(--color-hue-ai)",
      mobile: true,
      feature: "chat",
    },
  ],
} satisfies PageMeta;
