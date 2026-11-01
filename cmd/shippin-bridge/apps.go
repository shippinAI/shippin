package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"shippin.cloud/console/internal/apps"
)

func appsCmd(args []string) error {
	if len(args) == 0 {
		return errors.New(appsUsage)
	}
	switch args[0] {
	case "print":
		if len(args) != 2 {
			return errors.New(appsUsage)
		}
		return appsPrint(os.Stdout, args[1])
	case "install":
		if len(args) != 2 {
			return errors.New(appsUsage)
		}
		return appsInstall(args[1])
	case "update":
		if len(args) != 2 {
			return errors.New(appsUsage)
		}
		return appsUpdate(args[1])
	case "status":
		return appsStatus()
	case "open-desktop":
		return appsOpenDesktop()
	default:
		return errors.New(appsUsage)
	}
}

const appsUsage = `usage: shippin-bridge apps print <claude-desktop|claude-code|claude-code-apt|codex|chatgpt|grok|id-update>
       shippin-bridge apps install <claude-desktop|claude-code|claude-code-apt|codex|chatgpt|all>
       shippin-bridge apps update <claude-desktop|claude-code|claude-code-apt|codex|chatgpt>
       shippin-bridge apps status
       shippin-bridge apps open-desktop

print, install, and update use the vendor's own steps. Run them in your VM terminal (the
console terminal or your devspace), as your own Linux account. install ends in the vendor
sign-in. update upgrades an installed package and does not open a session. Apt steps use
sudo, which members have inside their own tenant VM. status never reads vendor credentials.`

func appsPrint(w io.Writer, id string) error {
	if id == "all" {
		for _, a := range apps.Catalog() {
			if a.ID == "open-desktop" {
				continue
			}
			fmt.Fprintf(w, "### %s\n", a.ID)
			if err := appsPrint(w, a.ID); err != nil {
				fmt.Fprintln(w, err)
			}
			fmt.Fprintln(w)
		}
		return nil
	}
	text, app, ok := apps.CopyTarget(id)
	if !ok || id == "open-desktop" || id == "open-desktop-update" {
		return fmt.Errorf("unknown app %q", id)
	}
	if text == "" {
		return fmt.Errorf("%s has no one-line install. Open %s", app.Title, app.DocsURL)
	}
	fmt.Fprintln(w, text)
	return nil
}

func appsUpdate(id string) error {
	if strings.HasSuffix(id, "-update") || id == "all" || id == "grok" {
		return errors.New(appsUsage)
	}
	text, app, ok := apps.CopyTarget(id + "-update")
	if !ok {
		return fmt.Errorf("unknown app %q", id)
	}
	if text == "" {
		return fmt.Errorf("%s has no update command. Open %s", app.Title, app.DocsURL)
	}
	fmt.Fprintf(os.Stderr, "Running the official %s update in this shell. Paste it in your VM terminal if this machine is not the tenant VM.\n", appLabel(app, id))
	cmd := exec.Command("bash", "-euo", "pipefail", "-c", text)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func appsInstall(id string) error {
	if id == "grok" {
		return appsPrint(os.Stderr, id)
	}
	if id == "all" {
		// The child bash of each install must see this, or the first sign-in swallows the rest.
		if err := os.Setenv("SHIPPIN_APPS_NO_LAUNCH", "1"); err != nil {
			return err
		}
		for _, one := range []string{"claude-code", "codex", "claude-desktop", "chatgpt"} {
			fmt.Println("==", one)
			if err := appsInstall(one); err != nil {
				return err
			}
		}
		fmt.Println("Installed. Open each app to sign in with your own account. Grok Bot: download from https://x.ai/bot")
		return nil
	}
	if strings.HasSuffix(id, "-update") {
		return errors.New(appsUsage)
	}
	text, app, ok := apps.CopyTarget(id)
	if !ok || text == "" {
		return appsPrint(os.Stderr, id)
	}
	fmt.Fprintf(os.Stderr, "Running the official %s install in this shell. Paste it in your VM terminal if this machine is not the tenant VM.\n", appLabel(app, id))
	cmd := exec.Command("bash", "-euo", "pipefail", "-c", text)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func appLabel(app apps.App, id string) string {
	if app.AltID != "" && (id == app.AltID || id == app.AltID+"-update") {
		return app.Title + " (apt)"
	}
	return app.Title
}

func appsStatus() error {
	fmt.Println("Signed-in: unknown. Shippin does not read vendor credentials.")
	c, err := loadCreds()
	if err != nil {
		return err
	}
	ids, err := tenantIDs(c)
	if err != nil {
		return err
	}
	for _, id := range ids {
		var view struct {
			StatusDetail string `json:"status_detail"`
			Blocks       []struct {
				Title string `json:"title"`
				Body  string `json:"body"`
			} `json:"blocks"`
		}
		if err := call(c, "GET", "/v1/tenants/"+id+"/apps", nil, &view); err != nil {
			return err
		}
		fmt.Printf("tenant %s\n", id)
		if view.StatusDetail != "" {
			fmt.Println(view.StatusDetail)
		}
		for _, b := range view.Blocks {
			line := b.Body
			if i := strings.LastIndex(b.Body, "\n"); i >= 0 {
				line = b.Body[i+1:]
			}
			fmt.Printf("  %s — %s\n", b.Title, line)
		}
	}
	return nil
}

func appsOpenDesktop() error {
	c, err := loadCreds()
	if err != nil {
		return err
	}
	ids, err := tenantIDs(c)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errors.New("no tenant on this session")
	}
	for _, id := range ids {
		fmt.Println(desktopURL(c.Server, id))
	}
	fmt.Println("Closing the browser tab leaves the VM desktop session logged in.")
	return nil
}

func desktopURL(server, tenantID string) string {
	return strings.TrimRight(server, "/") + "/t/" + tenantID + "/desktop"
}

func tenantIDs(c creds) ([]string, error) {
	ws, _, err := workspaces(c)
	if err != nil {
		return nil, err
	}
	var ids []string
	seen := map[string]bool{}
	for _, w := range ws {
		if w.TenantID != "" && !seen[w.TenantID] {
			seen[w.TenantID] = true
			ids = append(ids, w.TenantID)
		}
	}
	return ids, nil
}
