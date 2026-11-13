package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"shippin.cloud/console/internal/runfleet"
)

const usageText = `usage: shippin-run <command>

  mcp     stdio MCP server for runner tools
  serve   HTTP control plane (webhook, JSON API, and POST /mcp)
  sizes   print the runner size catalog as JSON

Environment:
  SHIPPIN_RUN_STATE_DIR                  fleet and job files (required)
  SHIPPIN_RUN_API_TOKEN                  bearer token for serve
  SHIPPIN_RUN_LISTEN                     listen address (default :8080)
  SHIPPIN_RUN_CONFIG                     optional JSON config
  SHIPPIN_RUN_GITHUB_APP_ID              GitHub App id
  SHIPPIN_RUN_GITHUB_PRIVATE_KEY_FILE    PEM private key
  SHIPPIN_RUN_GITHUB_INSTALLATION_ID     installation to call as
  SHIPPIN_RUN_GITHUB_TOKEN               installation token, instead of minting one
  SHIPPIN_RUN_GITHUB_WEBHOOK_SECRET      verifies X-Hub-Signature-256
  SHIPPIN_RUN_PROVISIONER                none, proxmox, or incus
  SHIPPIN_RUN_PROXMOX_API_URL            Proxmox API base, for health probes
  SHIPPIN_RUN_PROXMOX_TOKEN              Proxmox API token
  SHIPPIN_RUN_PROXMOX_NODE               Proxmox node name
  SHIPPIN_RUN_FREE_MINUTES               included linux-x64-2 minutes per month
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "install-url":
		url, err := installURL(strings.TrimSpace(os.Getenv("SHIPPIN_RUN_GITHUB_APP_SLUG")))
		if err != nil {
			fail(err)
		}
		fmt.Println(url)
	case "sizes":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(runfleet.DefaultSizes()); err != nil {
			fail(err)
		}
	case "mcp":
		svc, _, err := openFromEnv()
		if err != nil {
			fail(err)
		}
		if err := serveMCP(os.Stdin, os.Stdout, svc); err != nil {
			fail(err)
		}
	case "serve":
		svc, cfg, err := openFromEnv()
		if err != nil {
			fail(err)
		}
		if cfg.Token == "" {
			fail(fmt.Errorf("SHIPPIN_RUN_API_TOKEN is required"))
		}
		if err := http.ListenAndServe(cfg.Listen, newHandler(svc, cfg)); err != nil {
			fail(err)
		}
	default:
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(2)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "shippin-run:", err)
	os.Exit(1)
}
