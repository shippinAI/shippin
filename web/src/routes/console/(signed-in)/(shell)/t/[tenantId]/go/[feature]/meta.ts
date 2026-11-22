import { BarChart3, Bot, Globe, KeyRound, SquareTerminal } from "lucide-react";
import type { PageMeta } from "../../../../../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "tenant.terminal",
      section: "devspace",
      order: 70,
      label: "Terminal",
      icon: SquareTerminal,
      to: "go/terminal",
      hue: "var(--color-hue-workspaces)",
      feature: "workspaces",
    },
    {
      id: "tenant.connect",
      section: "devspace",
      order: 80,
      label: "Connect accounts",
      icon: KeyRound,
      to: "go/connect",
      hue: "var(--color-hue-secrets)",
      feature: "workspaces",
    },
    {
      id: "tenant.publish",
      section: "devspace",
      order: 90,
      label: "Publish",
      icon: Globe,
      to: "go/publish",
      hue: "var(--color-hue-edge)",
      feature: "workspaces",
    },
    {
      id: "tenant.usage",
      section: "devspace",
      order: 110,
      label: "Usage",
      icon: BarChart3,
      to: "go/usage",
      hue: "var(--color-hue-data)",
      feature: "workspaces",
    },
    {
      id: "tenant.ai",
      section: "devspace",
      order: 160,
      label: "AI & Agents",
      icon: Bot,
      to: "go/ai",
      hue: "var(--color-hue-ai)",
      feature: "ai",
    },
  ],
} satisfies PageMeta;
