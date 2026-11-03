# Claude in your devspace: terminal, Claude Desktop, phone

Claude Code runs inside your own devspace, signed in with your own Claude subscription (Pro or
Max is enough). You reach it three ways. All of them go through `shippin-bridge` after Shippin
sign-in: no static SSH keys, no open ports, certificates that expire in minutes.

## Once per computer

```bash
shippin-bridge login --server https://console.shippin.cloud   # device code, approve in the console
shippin-bridge ls                                             # your devspaces
```

## Terminal

```bash
shippin-bridge ssh dev        # then: claude
```

## Claude Desktop (and any tool that uses the system ssh)

```bash
shippin-bridge ssh-config dev >> ~/.ssh/config     # Windows: %USERPROFILE%\.ssh\config
ssh shippin-dev                                      # optional check
```

Put the block before any `Host *` section: ssh uses the first value it finds for each setting.

In Claude Desktop: **Code** tab, open the environment selector, **New environment**, **SSH**, then in
**Host** choose **Add SSH connection...**, enter `shippin-dev` as the SSH host, leave the key empty,
and pick a folder in the devspace. Desktop installs Claude Code in the devspace on first connect
and runs your sessions there.

How the block works: `Match host shippin-dev exec "shippin-bridge cert <id>"` runs before each
connection and keeps a short-lived certificate in `~/.shippin/bridge/ssh/` (reused while it has
more than 3 minutes left, so Desktop's reconnects don't ask the console every time);
`ProxyCommand shippin-bridge proxy <id>` carries the connection over the same TLS relay as
`shippin-bridge ssh`. The devspace's host key is pinned on first use in
`~/.shippin/bridge/known_hosts`. Signed out of the bridge, the block stops applying and ssh fails.

Requirements: `shippin-bridge` on your PATH (or at a path without spaces). On Windows, Desktop
uses the system ssh only with Microsoft's OpenSSH 9.4 or later; otherwise it uses its built-in
client, which does not read `~/.ssh/config`.

## Phone and web

Start Claude Code in the devspace with Remote Control on, and its sessions appear at
claude.ai/code and in the Claude app:

```bash
shippin-bridge ssh dev
claude remote-control --name "dev" --spawn worktree    # first run: sign in, confirm, trust the folder
```

Remote Control needs Claude Code talking to Anthropic directly: leave `ANTHROPIC_BASE_URL` unset.
