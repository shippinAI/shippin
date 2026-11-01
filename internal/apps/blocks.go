package apps

import (
	"fmt"
	"strings"

	"shippin.cloud/console/internal/chat"
)

// Status is what a tenant VM reported. SignedIn is always "unknown": Shippin does not read
// vendor credential files or keyrings.
type Status struct {
	Packages     map[string]string `json:"packages"`
	Users        []UserStatus      `json:"users"`
	XRDPLoopback bool              `json:"xrdp_loopback"`
	GrokDesktop  bool              `json:"grok_desktop"`
	KVM          bool              `json:"kvm"`
	SignedIn     string            `json:"signed_in"`
}

// UserStatus is one local account in the per-org uid range. Booleans are "the binary is
// executable", not "the vendor accepted a login".
type UserStatus struct {
	User          string `json:"user"`
	UID           int    `json:"uid"`
	Claude        bool   `json:"claude"`
	ClaudeDesktop bool   `json:"claude_desktop"`
	Codex         bool   `json:"codex"`
	ChatGPT       bool   `json:"chatgpt"`
}

// Blocks builds the chat cards for one tenant. st may be nil when the VM could not be read.
func Blocks(tenantID string, st *Status, detail string) []chat.Block {
	var out []chat.Block
	for _, a := range Catalog() {
		body := a.Summary
		if line := statusLine(a, st); line != "" {
			body += "\n" + line
		}
		if detail != "" && a.ID == "open-desktop" {
			body += "\n" + detail
		}
		card := chat.Block{Type: "card", Name: a.ID, Title: a.Title, Body: body}
		if a.ID == "open-desktop" {
			card.Items = append(card.Items, chat.Block{Type: "button", Name: a.ID, Label: "Open desktop", Href: "/t/" + tenantID + "/desktop"})
		}
		if a.Command != "" {
			card.Items = append(card.Items, chat.Block{Type: "button", Name: a.ID, Label: "Copy", Copy: a.Command})
		}
		if a.AltCommand != "" {
			label := a.AltLabel
			if label == "" {
				label = "Alternative"
			}
			card.Items = append(card.Items, chat.Block{Type: "button", Name: a.AltID, Label: label, Copy: a.AltCommand})
		}
		// A card keeps at most four buttons. When an alt install has its own update, that
		// button takes the slot and the default update stays available as `shippin apps update <id>`
		// and in the card note (Claude Code's native installer auto-updates).
		if a.Update != "" && a.AltUpdate == "" {
			card.Items = append(card.Items, chat.Block{Type: "button", Name: a.ID + "-update", Label: "Update", Copy: a.Update})
		}
		if a.AltUpdate != "" {
			label := a.AltUpdateLabel
			if label == "" {
				label = "Update"
			}
			card.Items = append(card.Items, chat.Block{Type: "button", Name: a.AltID + "-update", Label: label, Copy: a.AltUpdate})
		}
		if a.DocsURL != "" {
			label := a.DocsLabel
			if label == "" {
				label = "Official docs"
			}
			card.Items = append(card.Items, chat.Block{Type: "button", Name: a.ID + "-docs", Label: label, Href: a.DocsURL})
		}
		out = append(out, card)
	}
	return out
}

func statusLine(a App, st *Status) string {
	const unknown = "Signed-in: unknown."
	if st == nil {
		return "Status: unknown. " + unknown
	}
	switch a.ID {
	case "grok":
		if st.GrokDesktop {
			return "Status: installed (desktop entry present, version unknown). " + unknown
		}
		return "Status: not installed. " + unknown
	case "open-desktop":
		if st.XRDPLoopback {
			extra := ""
			if !st.KVM {
				extra = " /dev/kvm is not visible in the VM, so Claude Cowork cannot start until nested KVM is available."
			}
			return "Status: desktop ready (xrdp on 127.0.0.1:3389)." + extra
		}
		return "Status: desktop prerequisites not ready."
	case "codex":
		if anyUser(st, func(u UserStatus) bool { return u.Codex }) {
			return "Status: installed (binary present, version unknown). " + unknown
		}
		return "Status: not installed. " + unknown
	default:
		ver := ""
		if st.Packages != nil {
			ver = st.Packages[a.Package]
		}
		if ver != "" {
			return "Status: installed v" + ver + ". " + unknown
		}
		if a.ID == "claude-code" && anyUser(st, func(u UserStatus) bool { return u.Claude }) {
			return "Status: installed (binary present, version unknown). " + unknown
		}
		if a.ID == "claude-desktop" && anyUser(st, func(u UserStatus) bool { return u.ClaudeDesktop }) {
			return "Status: installed (binary present, version unknown). " + unknown
		}
		if a.ID == "chatgpt" && anyUser(st, func(u UserStatus) bool { return u.ChatGPT }) {
			return "Status: installed (binary present, version unknown). " + unknown
		}
		return "Status: not installed. " + unknown
	}
}

func anyUser(st *Status, pred func(UserStatus) bool) bool {
	for _, u := range st.Users {
		if pred(u) {
			return true
		}
	}
	return false
}

// HelperScript is the user-run `shippin` command installed on the tenant VM and printed by
// shippin-bridge. It only dispatches to the catalog commands.
func HelperScript() string {
	var b strings.Builder
	b.WriteString("#!/bin/bash\n")
	b.WriteString("# User-run helper. The commands are the vendors' own install and update steps.\n")
	b.WriteString("# Re-running an install is safe. Update upgrades an installed package and does not start a sign-in.\n")
	b.WriteString("# Apt steps use sudo. Members have sudo inside this tenant VM only.\n")
	b.WriteString("# Claude Code's default install is the native installer and does not use sudo.\n")
	b.WriteString("# This script does not read vendor credentials.\n")
	b.WriteString("set -euo pipefail\n")
	b.WriteString(`usage() {
  echo 'usage: shippin apps install <claude-desktop|claude-code|claude-code-apt|codex|chatgpt|all>' >&2
  echo '       shippin apps update  <claude-desktop|claude-code|claude-code-apt|codex|chatgpt>' >&2
  echo '       shippin apps print   <claude-desktop|claude-code|claude-code-apt|codex|chatgpt|grok|all|id-update>' >&2
  echo '       shippin apps status' >&2
  echo '       shippin apps open-desktop' >&2
  exit 2
}
print_one() {
  case "$1" in
`)
	for _, a := range Catalog() {
		if a.ID == "open-desktop" {
			continue
		}
		writeHelperCmd(&b, a.ID, a.Command, a)
		if a.Update != "" {
			writeHelperCmd(&b, a.ID+"-update", a.Update, a)
		}
		if a.AltCommand != "" {
			writeHelperCmd(&b, a.AltID, a.AltCommand, a)
		}
		if a.AltUpdate != "" {
			writeHelperCmd(&b, a.AltID+"-update", a.AltUpdate, a)
		}
	}
	b.WriteString(`    *) echo "unknown app: $1" >&2; return 1 ;;
  esac
}
install_one() {
  print_one "$1" | bash -euo pipefail
}
cmd=${1:-}; sub=${2:-}
[ "$cmd" = apps ] || usage
case "$sub" in
  print)
    id=${3:-}; [ -n "$id" ] || usage
    if [ "$id" = all ]; then
      for id in claude-desktop claude-code codex chatgpt grok; do echo "### $id"; print_one "$id" || true; echo; done
    else print_one "$id"; fi ;;
  install)
    id=${3:-}; [ -n "$id" ] || usage
    if [ "$id" = all ]; then
      export SHIPPIN_APPS_NO_LAUNCH=1
      for id in claude-code codex claude-desktop chatgpt; do echo "== $id"; install_one "$id"; done
      echo 'Installed. Open each app to sign in with your own account. Grok Bot: download from https://x.ai/bot'
    elif [ "$id" = grok ]; then print_one grok; exit 1
    else
      case "$id" in *-update) echo 'Updates are: shippin apps update <app>' >&2; exit 1 ;; esac
      install_one "$id"
    fi ;;
  update)
    id=${3:-}; [ -n "$id" ] || usage
    case "$id" in
      claude-desktop|claude-code|claude-code-apt|codex|chatgpt) install_one "$id-update" ;;
      *) echo "no update command for $id" >&2; exit 1 ;;
    esac ;;
  status) echo 'Signed-in: unknown (Shippin does not read vendor credentials).'; echo 'Run dpkg-query -W claude-desktop claude-code chatgpt and command -v claude codex chatgpt claude-desktop' ;;
  open-desktop) echo 'Open the console, then Open desktop. Closing the browser tab leaves the VM session logged in.' ;;
  *) usage ;;
esac
`)
	return b.String()
}

func writeHelperCmd(b *strings.Builder, id, cmd string, a App) {
	fmt.Fprintf(b, "    %s)\n", id)
	if cmd == "" {
		fmt.Fprintf(b, "      echo '%s has no one-line install. Open %s' >&2\n", a.Title, a.DocsURL)
		b.WriteString("      return 1\n      ;;\n")
		return
	}
	tag := shellTag(id)
	fmt.Fprintf(b, "      cat <<'SHIPPIN_CMD_%s'\n%s\nSHIPPIN_CMD_%s\n      ;;\n", tag, cmd, tag)
}

func shellTag(id string) string { return strings.ReplaceAll(id, "-", "_") }

// State is one app's install status with no command text. Operator views use this so a
// staff screen never offers a shell command.
type State struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// States is the status line for every catalog app.
func States(st *Status) []State {
	out := make([]State, 0, len(Catalog()))
	for _, a := range Catalog() {
		out = append(out, State{ID: a.ID, Title: a.Title, Status: statusLine(a, st)})
	}
	return out
}
