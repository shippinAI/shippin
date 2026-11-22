import { Bot } from "lucide-react";
import type { PageMeta } from "../../../../../../lib/router/meta";

export default {
  nav: [
    { id: "account.ai", section: "account", order: 60, label: "AI subscriptions", icon: Bot, hue: "var(--color-hue-ai)", feature: "ai" },
  ],
} satisfies PageMeta;
