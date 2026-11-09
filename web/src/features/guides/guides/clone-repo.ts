import type { Guide } from "../../../lib/guides/guide-types";

export const cloneRepoGuide: Guide = {
  id: "clone-repo",
  title: "Clone or import a GitHub repo",
  summary: "Bring an existing repository into this workspace.",
  steps: [
    { id: "auth", title: "Make sure GitHub is connected", body: "Cloning private repos needs your GitHub sign-in.", command: "gh auth status",
      explain: "If gh says you're logged in, you can clone anything your account can read.", link: { label: "Connect GitHub", to: "{connectUrl}" } },
    { id: "clone", title: "Clone the repo", body: "Replace OWNER/REPO with your repository, then press Enter.",
      command: "cd ~/code && gh repo clone OWNER/REPO", edit: true,
      explain: "gh downloaded the repository into ~/code/REPO with its full history." },
    { id: "enter", title: "Look around", body: "Move into the folder and see what's there.", command: "cd \"$(ls -td ~/code/*/ | head -1)\" && git log --oneline -5 && ls",
      explain: "You're in the newest folder under ~/code, with its last five commits listed." },
    { id: "import", title: "Prefer a managed import?", body: "Projects → Import from GitHub creates a workspace with a scoped, auto-refreshed token instead.",
      explain: "Imports use the Shippin GitHub app, so no personal token sits in the workspace.", link: { label: "Open Projects", to: "{projectsUrl}" } },
  ],
};
