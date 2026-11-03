import type { Guide } from "../../../lib/guides/guide-types";

export const adoptFolderGuide: Guide = {
  id: "adopt-folder",
  title: "Adopt a folder as a project",
  summary: "Turn a folder in ~/code into a Shippin project you can build, preview and publish.",
  steps: [
    { id: "list", title: "Find your folders", body: "Projects are folders under ~/code with a git repo or a package manifest.",
      command: "ls -d ~/code/*/", explain: "These are the folders Shippin can adopt." },
    { id: "init", title: "Make it a git repo (if it isn't)", body: "Replace my-app with your folder name, then press Enter.",
      command: "cd ~/code/my-app && git init -b main && git add -A && git commit -m 'First commit'", edit: true,
      explain: "The folder now has history, so Shippin can track and publish it." },
    { id: "adopt", title: "Adopt it", body: "Open the workspace's projects and choose “Adopt” next to the folder.",
      explain: "Adopting records the folder as a project; nothing in it is changed.", link: { label: "Open Projects", to: "{projectsUrl}" } },
  ],
};
