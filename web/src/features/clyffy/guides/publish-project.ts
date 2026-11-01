import type { Guide } from "../../../lib/guides/guide-types";

export const publishProjectGuide: Guide = {
  id: "publish-project",
  title: "Publish a project",
  summary: "Push your work to GitHub and put a public site online.",
  steps: [
    { id: "status", title: "Check for unsaved work", body: "See what changed since your last commit.", command: "git status --short",
      explain: "Each line is a changed file. An empty list means everything is committed." },
    { id: "commit", title: "Commit it", body: "Edit the message, then press Enter.", command: "git add -A && git commit -m 'Describe your change'", edit: true,
      explain: "Your changes are saved as one commit on this branch." },
    { id: "sync", title: "Create the GitHub repo & sync", body: "On the project page, “Create GitHub repo & sync” makes a private repo and pushes.",
      explain: "Shippin pushed through its GitHub app; history is scanned for secrets before it goes out.", link: { label: "Open Projects", to: "{projectsUrl}" } },
    { id: "publish", title: "Publish", body: "Use “Publish” on the project to freeze a release at a public address. You can roll back or unpublish anytime.",
      explain: "A frozen copy now serves at its public URL; editing the workspace doesn't change it until you publish again.", link: { label: "Open Projects", to: "{projectsUrl}" } },
  ],
};
