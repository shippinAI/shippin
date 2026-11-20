import { FolderGit2 } from "lucide-react";
import type { PageMeta } from "../../../../../../../lib/router/meta";

export default {
  nav: [
    { id: "tenant.projects", section: "devspace", order: 20, label: "Projects", icon: FolderGit2, hue: "var(--color-hue-workspaces)", feature: "workspaces" },
  ],
} satisfies PageMeta;
