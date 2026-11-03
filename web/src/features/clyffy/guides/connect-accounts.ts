import type { Guide } from "../../../lib/guides/guide-types";

export const connectAccountsGuide: Guide = {
  id: "connect-accounts",
  title: "Connect your accounts",
  summary: "Sign GitHub, Claude, Codex and Grok in once; the workspace keeps them.",
  steps: [
    { id: "open-cards", title: "Open Connect accounts", body: "Each provider has a card. Pick “Sign in with browser” for a one-time code, or paste a token.",
      explain: "Sign-ins run inside this workspace. Tokens go to Vaultix and come back on every start.",
      link: { label: "Open Connect accounts", to: "{connectUrl}" } },
    { id: "gh-status", title: "Check GitHub", body: "Ask the GitHub CLI who you are signed in as.", command: "gh auth status",
      explain: "gh printed the account and token scopes it uses. “not logged in” means the GitHub card still needs connecting." },
    { id: "ai-status", title: "Check the AI CLIs", body: "Each CLI reports its own sign-in.", command: "claude auth status --text; codex login status",
      explain: "Both commands only report status. Anything “not logged in” can be connected from its card." },
  ],
};
