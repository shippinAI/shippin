// Mirrors the server's repository rule (hostd.ValidRepo); hosts come from GET /v1/templates.
const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

export function repoPattern(hosts: string[]): RegExp {
  const h = hosts.map(esc).join("|");
  return new RegExp(`^https://(${h})/[A-Za-z0-9][A-Za-z0-9_.-]{0,99}/[A-Za-z0-9][A-Za-z0-9_.-]{0,99}?(\\.git)?/?$`);
}

export const repoHint = (hosts: string[]) => `Use a public https URL on ${hosts.join(", ")}.`;
