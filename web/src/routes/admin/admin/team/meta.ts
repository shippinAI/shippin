import { UsersRound } from "lucide-react";
import type { PageMeta } from "../../../../lib/router/meta";

export default {
  nav: [
    {
      id: "admin.team",
      section: "admin.people",
      order: 10,
      label: "Team",
      icon: UsersRound,
      hue: "var(--color-warning)",
      cap: "team.read",
    },
  ],
} satisfies PageMeta;
