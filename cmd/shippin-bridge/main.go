// Command shippin-bridge is THE way into a Shippin devspace: sign in once with Nomen (device
// code), then open a shell, run commands or serve the devspace to a local AI over MCP. Access is
// a short-lived SSH certificate per workspace, relayed through the console; nothing is opened on
// your machine and no long-lived key is kept.
package main

import (
	"fmt"
	"os"
)

const usage = `usage: shippin-bridge <command>

  login [--server https://console.example]  sign in with Nomen (device code)
  logout                                    forget this machine's bridge session
  ls                                        list the workspaces you can open
  ssh <workspace>                           interactive shell
  exec <workspace> -- <command...>          run one command
  proxy <workspace>                         byte stream for ssh ProxyCommand
  ssh-config <workspace>                    OpenSSH config block (Claude Desktop, editors, plain ssh)
  cert <workspace id>                       refresh the short-lived certificate ssh-config uses
  mcp                                       stdio MCP server for local AIs
  apps print|install|update|status|open-desktop  official AI app commands
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	args := os.Args[2:]
	switch os.Args[1] {
	case "login":
		err = login(args)
	case "logout":
		err = logout()
	case "ls":
		err = list()
	case "ssh":
		err = shell(args)
	case "exec":
		err = execCmd(args)
	case "proxy":
		err = proxy(args)
	case "ssh-config":
		err = sshConfig(args)
	case "cert":
		err = certCmd(args)
	case "mcp":
		err = serveMCP(os.Stdin, os.Stdout)
	case "apps":
		err = appsCmd(args)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "shippin-bridge:", err)
		os.Exit(1)
	}
}
