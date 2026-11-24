// Package apps is the official-install catalog for AI desktop and CLI apps.
// Commands are the vendors' own steps (pinned by the fingerprint they publish) plus the
// vendor sign-in at the end. Shippin never stores a vendor credential and never sets
// ANTHROPIC_API_KEY or ANTHROPIC_BASE_URL: Claude Remote Control and Dispatch break behind a gateway.
package apps

import "strings"

// ClaudeKeyFingerprint is the fingerprint Anthropic prints for both the Claude Code and
// Claude Desktop apt signing keys.
// https://code.claude.com/docs/en/setup
// https://code.claude.com/docs/en/desktop-linux
const ClaudeKeyFingerprint = "31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE"

// App is one card. Command is empty when there is no official one-line install to copy.
// Update is the vendor's upgrade step for that default install. It does not launch a sign-in.
// AltCommand is a second official install on the same card (Claude Code's apt repo).
type App struct {
	ID             string `json:"id"` // claude-desktop, claude-code, codex, chatgpt, grok, open-desktop
	Title          string `json:"title"`
	Summary        string `json:"summary"` // what the command does, including which update path each install uses
	Command        string `json:"command"` // exact paste text; empty when the user must use the vendor's download page
	Update         string `json:"update,omitempty"`
	AltID          string `json:"alt_id,omitempty"`
	AltLabel       string `json:"alt_label,omitempty"`
	AltCommand     string `json:"alt_command,omitempty"`
	AltUpdate      string `json:"alt_update,omitempty"`
	AltUpdateLabel string `json:"alt_update_label,omitempty"`
	DocsURL        string `json:"docs_url"`
	DocsLabel      string `json:"docs_label"`
	Where          string `json:"where"`   // terminal | desktop | download
	Package        string `json:"package"` // dpkg name when the official package has one
}

// Catalog is the fixed list the console, the chat cards and shippin-bridge share.
func Catalog() []App {
	return []App{
		{
			ID: "claude-desktop", Title: "Claude Desktop",
			Summary: "Installs Claude Desktop from Anthropic's signed apt repo and opens it so you can sign in with your own Claude account. Paste this in the VM desktop terminal. Update upgrades only this package.",
			Command: claudeDesktopCmd, Update: claudeDesktopUpdate,
			DocsURL: "https://code.claude.com/docs/en/desktop-linux", DocsLabel: "Desktop docs",
			Where: "desktop", Package: "claude-desktop",
		},
		{
			ID: "claude-code", Title: "Claude Code",
			Summary: "Copy runs Anthropic's recommended native installer (curl -fsSL https://claude.ai/install.sh | bash) and then claude so you can sign in. Native installations automatically update in the background to keep you on the latest version. Claude Code checks for updates on startup and periodically while running. Updates download and install in the background, then take effect the next time you start Claude Code. To apply an update immediately without waiting for the next background check, run claude update. Install with apt instead uses the signed stable apt repo and checks fingerprint " + ClaudeKeyFingerprint + ". Apt installations do not auto-update by default. apt, dnf, and apk continue to require a manual upgrade because those commands need elevated privileges. To upgrade later, run sudo apt update && sudo apt upgrade claude-code. Update apt upgrades only the claude-code package (apt-get install -y --only-upgrade) so a paste does not prompt.",
			Command: claudeCodeCmd, Update: claudeCodeNativeUpdate,
			AltID: "claude-code-apt", AltLabel: "Install with apt instead", AltCommand: claudeCodeAptCmd,
			AltUpdate: claudeCodeAptUpdate, AltUpdateLabel: "Update apt",
			DocsURL: "https://code.claude.com/docs/en/setup", DocsLabel: "Claude Code docs",
			Where: "terminal", Package: "claude-code",
		},
		{
			ID: "codex", Title: "Codex",
			Summary: "Installs the official Codex CLI, the terminal client in the same family as the ChatGPT desktop app, and starts it so you can sign in with ChatGPT. Update re-runs the installer and does not open a session.",
			Command: codexCmd, Update: codexUpdate,
			DocsURL: "https://developers.openai.com/codex/cli/", DocsLabel: "Codex CLI docs",
			Where: "terminal",
		},
		{
			ID: "chatgpt", Title: "ChatGPT",
			Summary: "Installs the official ChatGPT desktop app (graphical Codex) for this machine's architecture and opens it so you can sign in. An existing install is left alone. Computer Use is not in the Linux preview. Paste this in the VM desktop terminal.",
			Command: chatgptCmd, Update: chatgptUpdate,
			DocsURL: "https://learn.chatgpt.com/docs/linux/linux-app", DocsLabel: "ChatGPT Linux docs",
			Where: "desktop", Package: "chatgpt",
		},
		{
			ID: "grok", Title: "Grok Bot",
			Summary: "Grok Bot has an official Linux desktop build (.deb, .rpm, AppImage). xAI's docs say to download it from their page. There is no vendor-published one-line install or signing fingerprint, so Shippin does not invent one.",
			DocsURL: "https://x.ai/bot", DocsLabel: "Grok Bot downloads",
			Where: "download",
		},
		{
			ID: "open-desktop", Title: "Open desktop",
			Summary: "Opens this org's VM desktop in the browser. You sign in at the VM's own login screen. Closing the tab disconnects the viewer and leaves the desktop session running.",
			DocsURL: "https://manpages.debian.org/testing/xrdp/sesman.ini.5.en.html", DocsLabel: "Why the session stays",
			Where: "desktop",
		},
	}
}

// ByID returns the catalog entry or false.
func ByID(id string) (App, bool) {
	for _, a := range Catalog() {
		if a.ID == id {
			return a, true
		}
	}
	return App{}, false
}

// OfficialCommands is the exact clipboard allowlist (install and update pastes).
func OfficialCommands() []string {
	var out []string
	for _, a := range Catalog() {
		for _, c := range []string{a.Command, a.Update, a.AltCommand, a.AltUpdate} {
			if c != "" {
				out = append(out, c)
			}
		}
	}
	return out
}

// CopyTarget resolves a catalog id, "<id>-update", or an alt id such as "claude-code-apt".
// found is false for an unknown id. text is empty when that app has nothing to copy.
func CopyTarget(id string) (text string, app App, found bool) {
	for _, a := range Catalog() {
		switch id {
		case a.ID:
			return a.Command, a, true
		case a.ID + "-update":
			return a.Update, a, true
		}
		if a.AltID != "" && (id == a.AltID || id == a.AltID+"-update") {
			if id == a.AltID {
				return a.AltCommand, a, true
			}
			return a.AltUpdate, a, true
		}
	}
	return "", App{}, false
}

// IsOfficialCommand reports whether s is one of the catalog commands, byte for byte.
func IsOfficialCommand(s string) bool {
	for _, c := range OfficialCommands() {
		if s == c {
			return true
		}
	}
	return false
}

func init() {
	// Registered here so chat.Validate can keep a Copy button only for these strings.
	// A nil allowlist (tests that don't import this package) rejects every copy.
	registerCopyAllowlist()
}

// noLaunch is set by `shippin apps install all` so one interactive app does not swallow the rest.
const noLaunch = `${SHIPPIN_APPS_NO_LAUNCH:-}`

var claudeDesktopCmd = strings.TrimSpace(`
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
sudo apt-get update -qq
sudo apt-get install -y curl gnupg
sudo curl -fsSLo /usr/share/keyrings/claude-desktop-archive-keyring.asc https://downloads.claude.ai/claude-desktop/key.asc
gpg --show-keys /usr/share/keyrings/claude-desktop-archive-keyring.asc | grep -q '31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE'
printf '%s\n' 'deb [arch=amd64,arm64 signed-by=/usr/share/keyrings/claude-desktop-archive-keyring.asc] https://downloads.claude.ai/claude-desktop/apt/stable stable main' | sudo tee /etc/apt/sources.list.d/claude-desktop.list >/dev/null
sudo apt-get update -qq
sudo apt-get install -y claude-desktop
if [ -z "` + noLaunch + `" ] && [ -n "${DISPLAY:-}" ]; then
  claude-desktop
elif [ -z "` + noLaunch + `" ]; then
  echo 'Claude Desktop is installed. Open the VM desktop and launch Claude Desktop to sign in with your own Claude account.'
fi
`)

// claudeCodeCmd is the native installer marked "Native Install (Recommended)" on
// https://code.claude.com/docs/en/setup (re-read 23 Oct 2026):
// curl -fsSL https://claude.ai/install.sh | bash
// The page then says to log in by running claude. ~/.local/bin is prepended because that
// page places the launcher at ~/.local/bin/claude and the installer does not update this shell's PATH.
// This command does not use sudo.
var claudeCodeCmd = strings.TrimSpace(`
set -euo pipefail
curl -fsSL https://claude.ai/install.sh | bash
if [ -z "` + noLaunch + `" ]; then
  export PATH="${HOME}/.local/bin:${PATH:-}"
  exec claude
fi
`)

// claudeCodeNativeUpdate is the immediate native update. The setup page says native
// installations automatically update in the background, and: "To apply an update immediately
// without waiting for the next background check, run: claude update".
var claudeCodeNativeUpdate = strings.TrimSpace(`
set -euo pipefail
echo 'Native installations automatically update in the background to keep you on the latest version.'
echo 'Claude Code checks for updates on startup and periodically while running. Updates download and install in the background, then take effect the next time you start Claude Code.'
echo 'To apply an update immediately without waiting for the next background check, run: claude update'
claude update
`)

// claudeCodeAptCmd is the signed apt method on the same page (stable channel).
// The fingerprint check is the one the page tells you to confirm before continuing.
var claudeCodeAptCmd = strings.TrimSpace(`
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
sudo apt-get install -y curl gnupg
sudo install -d -m 0755 /etc/apt/keyrings
sudo curl -fsSL https://downloads.claude.ai/keys/claude-code.asc -o /etc/apt/keyrings/claude-code.asc
gpg --show-keys /etc/apt/keyrings/claude-code.asc | grep -q '31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE'
printf '%s\n' 'deb [signed-by=/etc/apt/keyrings/claude-code.asc] https://downloads.claude.ai/claude-code/apt/stable stable main' | sudo tee /etc/apt/sources.list.d/claude-code.list >/dev/null
sudo apt-get update -qq
sudo apt-get install -y claude-code
if [ -z "` + noLaunch + `" ]; then
  exec claude
fi
`)

var codexCmd = strings.TrimSpace(`
set -euo pipefail
curl -fsSL https://chatgpt.com/codex/install.sh | sh
if [ -z "` + noLaunch + `" ]; then
  exec codex
fi
`)

// codexUpdate re-runs the official installer and does not open a session.
var codexUpdate = strings.TrimSpace(`
set -euo pipefail
curl -fsSL https://chatgpt.com/codex/install.sh | sh
`)

// aptOnlyUpgrade is the package-scoped update pasted from a card.
// The Claude Desktop page shows a full-system `apt upgrade`; this upgrades one package
// so a paste on the shared VM does not upgrade every package. -y avoids an apt prompt.
func aptOnlyUpgrade(pkg string) string {
	return strings.TrimSpace(`
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
sudo apt-get update -qq
sudo apt-get install -y --only-upgrade ` + pkg + `
`)
}

var claudeDesktopUpdate = aptOnlyUpgrade("claude-desktop")
var claudeCodeAptUpdate = aptOnlyUpgrade("claude-code")
var chatgptUpdate = aptOnlyUpgrade("chatgpt")

// chatgptCmd is the Debian path on https://learn.chatgpt.com/docs/linux/linux-app
// (re-read 23 Oct 2026). That page shows no checksum and no signing-key fingerprint,
// so this command does not invent one. uname -m selects chatgpt_amd64.deb or
// chatgpt_arm64.deb under /deb/latest/. Installing that .deb configures OpenAI's
// signed apt repository. An already-installed version is left alone.
// The page's optional Wayland flag is omitted: the VM desktop is X11/xrdp.
// Computer Use is not in the Linux preview and is not part of this command.
// Members have sudo inside this tenant VM, which the apt step uses.
var chatgptCmd = strings.TrimSpace(`
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
if dpkg-query -W -f '${Status}' chatgpt 2>/dev/null | grep -q 'install ok installed'; then
  echo 'ChatGPT is already installed. This paste leaves that version alone. Use Update to upgrade it.'
else
  case "$(uname -m)" in
    x86_64) deb=chatgpt_amd64.deb ;;
    aarch64|arm64) deb=chatgpt_arm64.deb ;;
    *) echo "ChatGPT publishes amd64 and arm64 packages only (this machine is $(uname -m))." >&2; exit 1 ;;
  esac
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  curl -fsSL "https://persistent.oaistatic.com/codex-app-prod/linux/deb/latest/$deb" -o "$tmp/$deb"
  sudo apt-get install -y "$tmp/$deb"
fi
if [ -z "` + noLaunch + `" ] && [ -n "${DISPLAY:-}" ]; then
  chatgpt
elif [ -z "` + noLaunch + `" ]; then
  echo 'ChatGPT is installed. Open the VM desktop and run chatgpt to sign in with your own ChatGPT account.'
fi
`)
