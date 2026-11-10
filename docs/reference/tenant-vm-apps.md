# AI apps on a tenant VM

Members install Claude, Codex, and ChatGPT themselves. The console shows the vendor's command, a Copy button, an Update button where the vendor documents an upgrade, and one line that says what it does. Pasting the install command in the member's own terminal (the console web terminal or a devspace terminal, as their own Linux account) installs the app and starts the vendor's sign-in. The subscription is the member's. Shippin does not store vendor credentials and does not set `ANTHROPIC_API_KEY` or `ANTHROPIC_BASE_URL`. Org members have sudo in their own tenant VM, which the apt steps use.

The same cards are in chat, on the workspace terminal, on the devspace terminal, and under tenant settings. Open desktop is a link to `/t/<tenant>/desktop`.

## What each card does

| Card | Official Linux build | What the command does |
| --- | --- | --- |
| Claude Desktop | Yes. Apt repo, package `claude-desktop`. [Desktop on Linux](https://code.claude.com/docs/en/desktop-linux) | Installs from Anthropic's signed apt repo (fingerprint `31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE`) and opens Claude Desktop when `DISPLAY` is set. Paste it in the VM desktop terminal. Update runs `apt-get update` and `apt-get install -y --only-upgrade claude-desktop`. The page shows a full-system `apt upgrade`; the button upgrades only this package so one paste does not upgrade the whole VM. Update does not launch the app. |
| Claude Code | Yes. Native installer is the recommended one. Apt is the signed alternative. [Setup](https://code.claude.com/docs/en/setup) | Copy runs `curl -fsSL https://claude.ai/install.sh \| bash` and then `claude`. The page calls that "Native Install (Recommended)". The same shell prepends `~/.local/bin`, which is where that page says the launcher lives. Native installations automatically update in the background to keep you on the latest version. Claude Code checks for updates on startup and periodically while running. Updates download and install in the background, then take effect the next time you start Claude Code. To apply an update immediately without waiting for the next background check, run `claude update` (`shippin apps update claude-code`). Install with apt instead checks fingerprint `31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE` and installs from the stable apt repo. Apt installations do not auto-update by default. apt, dnf, and apk continue to require a manual upgrade because those commands need elevated privileges. To upgrade later, run `sudo apt update && sudo apt upgrade claude-code`. Update apt runs that for the `claude-code` package only (`apt-get install -y --only-upgrade claude-code`) so a paste does not prompt. |
| Codex | CLI, same product family as the ChatGPT desktop app. [Codex CLI](https://developers.openai.com/codex/cli/) | Runs `curl -fsSL https://chatgpt.com/codex/install.sh \| sh` and then `codex`. The vendor command does not use sudo, and this one does not add it. There is no separate Linux Codex desktop package. Graphical Codex on Linux is the ChatGPT desktop app (`codex-app-prod`). Update re-runs that installer and does not open a session. |
| ChatGPT | Yes, Linux preview (Ubuntu 24.04/26.04, Debian 13, Fedora 43/44, Arch; x64 and ARM64). [Linux app](https://learn.chatgpt.com/docs/linux/linux-app) | One paste for Debian/Ubuntu. `uname -m` picks `chatgpt_amd64.deb` (`x86_64`) or `chatgpt_arm64.deb` (`aarch64` or `arm64`) from `https://persistent.oaistatic.com/codex-app-prod/linux/deb/latest/`, downloads it to a temp dir, and runs `sudo apt-get install -y` on it (the page writes `sudo apt install ./chatgpt_amd64.deb`; `-y` and `DEBIAN_FRONTEND=noninteractive` keep a paste from prompting). Then it launches `chatgpt` when `DISPLAY` is set. If `dpkg` already shows `chatgpt` installed, the paste leaves that version alone and still launches it. Computer Use is not in the Linux preview. The page's optional `chatgpt --ozone-platform=wayland` is not used: the VM desktop is X11/xrdp. The page also documents Fedora and Arch; tenant VMs are Debian, so the card is the Debian path. |
| Grok Bot | Yes (.deb, .rpm, AppImage). [Get started](https://docs.x.ai/grok-bot/get-started), downloads at [x.ai/bot](https://x.ai/bot) | No vendor one-line install and no published fingerprint, so the card is a link. Shippin does not invent a command. |
| Open desktop | — | Opens the VM desktop in the browser. You type the Linux password at the VM's own login screen. |

Re-running an install is safe. ChatGPT's install paste does not upgrade a package that is already installed. `shippin apps install all` (and `shippin-bridge apps install all`) sets `SHIPPIN_APPS_NO_LAUNCH` so one sign-in does not swallow the later installs. A single pasted install does not set that variable, so it launches the vendor sign-in. Update is a second button on the same card. It is not part of `install all`.

The ChatGPT Linux page does not show a checksum or a Debian signing-key fingerprint. The command does not invent one, and it does not check `/deb/latest/*.deb` against the stable Packages index (those can diverge). Installing the official `.deb` configures OpenAI's signed apt repository. Later upgrades use that repo. A third-party RPM fingerprint is not pinned.

Signed-in is always **unknown**. Status is "not installed", "installed vX" from `dpkg-query`, or "installed (binary present, version unknown)" from the executable bit on fixed paths (`~/.local/bin`, `/usr/bin`, `/usr/local/bin` only). The probe does not use sudo, does not run vendor CLIs, and does not read `~/.claude.json`, keyrings, or credential files. Accounts outside uid 10000–59999 are ignored.

## Commands

In the VM terminal, either paste the command from the card or use the helper the desktop prepare step installs:

```bash
shippin apps print claude-code
shippin apps install claude-code
shippin apps update claude-code
shippin apps install claude-code-apt
shippin apps update claude-code-apt
shippin apps install all
shippin apps status
shippin apps open-desktop
```

`shippin apps print chatgpt-update` prints the update paste. `shippin apps update chatgpt` runs it.

From a laptop, `shippin-bridge` prints the same text and can call the console for status:

```bash
shippin-bridge apps print claude-code
shippin-bridge apps print chatgpt-update
shippin-bridge apps update chatgpt
shippin-bridge apps status
shippin-bridge apps open-desktop
```

`print` does not need the helper. `install` and `update` run in the shell you are in, so run them on the VM. `install claude-code` is the native installer and does not use sudo. `install claude-code-apt` and the other apt steps use `sudo`. Members have sudo inside their own tenant VM (see below). The Codex CLI paste is the vendor's `curl | sh` and does not add sudo. `update claude-code` runs `claude update`. `update claude-code-apt` upgrades the apt package.

## Open desktop

The page at `/t/<tenant>/desktop` is behind the console login. The browser speaks a WebSocket to the console, and the console opens a fixed pipe to `127.0.0.1:3389` inside that tenant's VM only (`incus exec` of a script with the address and port baked in). No new public port. Closing the tab closes the viewer.

The desktop service is xrdp with xorgxrdp. Debian's [sesman.ini(5)](https://manpages.debian.org/testing/xrdp/sesman.ini.5.en.html) says `KillDisconnected=true` ends the session after a disconnect, and that this applies to xorgxrdp sessions. Persistence is `KillDisconnected=false` (and `IdleTimeLimit=0`). `Policy` is left alone when it is already set, so an existing session policy is not rewritten.

This build carries that authenticated pipe. It does not draw the desktop picture in the page. IronRDP's web client needs RDCleanPath, and forcing the existing listener to `security_layer=rdp` would weaken a VM that was set up by hand. Which browser client to use is an open decision.

## What the host still does

`POST /v1/tenants/<id>/desktop/prepare` (owners and admins) and the operator action with the same name install desktop prerequisites inside the tenant VM only:

- XFCE, xrdp, xorgxrdp, gnome-keyring, and the Cowork packages the Claude Desktop package recommends (`qemu-system-x86`, `ovmf`, `virtiofsd`)
- xrdp bound to `127.0.0.1:3389`
- sesman left running across disconnects
- keyring PAM lines, locker and DPMS nudged off at login, sleep targets masked
- `vhost_vsock` and the `kvm` group for uids 10000–59999 (it does not create accounts)
- `/usr/local/bin/shippin`, the user-run helper

It refuses a container, a workspace, and the control-plane project. The request body cannot choose a command. Apt uses `--force-confold` so a package upgrade does not overwrite a hand-written config. xrdp is restarted only when the listen address, the port, or `KillDisconnected` actually changed, so a VM that is already on loopback with sessions kept is not bounced. Vendor apt sources are not rewritten. The script does not install `claude-desktop`, `claude-code`, `chatgpt`, or Codex.

Any member can read status and copy a command. Copy is audited as `apps.command_copied` (the command is still shown if the audit write fails). The detail is `tvm-<tenant>` plus the button name, so an update copy (`chatgpt-update`) is distinct from the install copy. Prepare and opening the desktop pipe are audited first (`apps.desktop_prepare`, `apps.desktop_open`) and refused if the audit cannot be written.

## Deploy and roll back

There is no database migration. Roll back the same way as any other console and hostd release.

Do not deploy from this change. Deployment waits for the operator's approval. Do not run it on the control-plane host.

## Sudo in the tenant VM

Decided: org members have sudo in their org's tenant VM, the same way members have sudo on a VM in other multi-tenant clouds. The Claude and ChatGPT card commands, their Update buttons, and the `shippin` helper that runs those steps all use `sudo` where the vendor's apt instructions do. The helper states that members have sudo inside this tenant VM.

Sudo is root inside that VM only. The per-tenant VM boundary is what protects other tenants and the host. hostd and the control-plane host stay staff-only. Desktop prerequisite packages stay a hostd action (VM-only, allowlisted) because they prepare the shared desktop session.

Follow-up for `docs/design/identity-local-accounts.md` on branch `cursor/devspace-foundation-8c87` (PR #2, not edited here): that doc says members do not get sudo on the VM (sudo only inside their containers). Update it to say members get sudo in their tenant VM, with sudo scoped to that VM only, so the tenant VM isolation boundary and the hostd/the reference host host controls stay staff-only.

## Decisions still open

- **Browser pixels.** The pipe is in place. The in-page picture is not.

## Already decided

- **Base branch.** This is based on `b2-release`, not `main`. `main` is an ancestor of `b2-release`. The console web terminal these cards sit on exists on `b2-release`. `approve-p1-only` is further ahead and is about publish gates.
- **ChatGPT integrity.** The Linux app page (re-read 23 Oct 2026) shows no verification step. The SHA256 check against the stable Packages index is dropped. The signed repo comes from installing the official `.deb`. No fingerprint is invented.
- **Sudo.** Members have it inside their own tenant VM. See above.
- **Claude Code installer.** The default paste is the recommended native command on https://code.claude.com/docs/en/setup (re-read 23 Oct 2026): `curl -fsSL https://claude.ai/install.sh | bash`, then `claude`. The fingerprint-pinned stable apt install is the "Install with apt instead" button on the same card, with Update apt for `sudo apt update && sudo apt upgrade claude-code`. Native installs auto-update in the background; apt does not.
