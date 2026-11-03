/**
 * The downloadable ~/.wezterm.lua for one workspace: an ssh domain using the WezTerm multiplexer
 * (the image ships wezterm, so `wezterm connect` keeps tabs alive across disconnects) and a plain
 * ssh fallback. Both start in the workspace's Zellij session via shippin-term.
 */
export type WezTermTarget = { workspace: string; host: string; port: number; user: string };

/** Lua-safe domain name: letters, digits, dash. */
export const domainName = (ws: string) => `shippin-${ws.toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^-+|-+$/g, "") || "workspace"}`;

const lua = (s: string) => `'${s.replace(/\\/g, "\\\\").replace(/'/g, "\\'").replace(/[\r\n]/g, "")}'`;

export function weztermConfig(t: WezTermTarget): string {
  const name = domainName(t.workspace);
  const addr = lua(`${t.host}:${Math.trunc(t.port) || 22}`);
  return `-- WezTerm config for the Shippin workspace "${t.workspace.replace(/[\r\n]/g, "")}".
-- Save as ~/.wezterm.lua (or merge the ssh_domains entries), then run:
--   wezterm connect ${name}
-- Uses your SSH key (add it under Account → SSH keys).
local wezterm = require 'wezterm'
local config = wezterm.config_builder()

config.ssh_domains = config.ssh_domains or {}
table.insert(config.ssh_domains, {
  -- WezTerm mux on the workspace: tabs and panes survive a dropped connection.
  name = ${lua(name)},
  remote_address = ${addr},
  username = ${lua(t.user)},
  multiplexing = 'WezTerm',
  remote_wezterm_path = '/usr/bin/wezterm',
  default_prog = { '/usr/local/bin/shippin-term' },
})
table.insert(config.ssh_domains, {
  -- Plain ssh (no remote WezTerm needed); opens the same Zellij session "shippin".
  name = ${lua(`${name}-ssh`)},
  remote_address = ${addr},
  username = ${lua(t.user)},
  multiplexing = 'None',
  default_prog = { '/usr/local/bin/shippin-term' },
})

return config
`;
}
