import { Bot } from "lucide-react";
import type { PageMeta } from "../../../../../../lib/router/meta";

export default {
  nav: [
    { id: "account.ai-access", section: "account", order: 30, label: "AI access", icon: Bot, hue: "var(--color-hue-ai)", cap: "account.local" },
  ],
} satisfies PageMeta;
