package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// login runs the device-code sign-in: show the code, wait for approval in the console (signed in
// with Nomen), store the bridge token in the OS keychain.
func login(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	server := fs.String("server", os.Getenv("SHIPPIN_URL"), "console URL")
	name := fs.String("name", hostLabel(), "how this machine appears in your sessions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !strings.HasPrefix(*server, "https://") {
		return errors.New("--server https://<console host> (or SHIPPIN_URL) is required")
	}
	c := creds{Server: strings.TrimRight(*server, "/")}
	var d struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
		URI        string `json:"verification_uri_complete"`
		ExpiresIn  int    `json:"expires_in"`
		Interval   int    `json:"interval"`
	}
	if err := call(c, "POST", "/v1/bridge/device", map[string]string{"client_name": *name}, &d); err != nil {
		return err
	}
	if d.ExpiresIn <= 0 || d.Interval <= 0 {
		return errors.New("the server returned invalid sign-in timing; try again")
	}
	fmt.Fprintf(os.Stderr, "Open %s\nand confirm the code %s (signed in to Shippin).\nKeep this command running. After Allow, it connects automatically. This link expires in %s.\n", d.URI, d.UserCode, time.Duration(d.ExpiresIn)*time.Second)
	deadline := time.Now().Add(time.Duration(d.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(d.Interval) * time.Second)
		var t struct {
			Token   string            `json:"access_token"`
			Account map[string]string `json:"account"`
		}
		err := call(c, "POST", "/v1/bridge/token", map[string]string{"device_code": d.DeviceCode}, &t)
		var ae *apiError
		if errors.As(err, &ae) && ae.Msg == "authorization_pending" {
			continue
		}
		if err != nil {
			return loginError(err)
		}
		c.Token = t.Token
		if err := saveCreds(c); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Signed in as %s (account %s).\n", t.Account["email"], t.Account["id"])
		return nil
	}
	return errors.New("the code expired; run login again")
}

func loginError(err error) error {
	var ae *apiError
	if errors.As(err, &ae) {
		switch ae.Msg {
		case "expired_token":
			return errors.New("this sign-in link expired; run login again and approve the new code")
		case "access_denied":
			return errors.New("sign-in was denied; no bridge access was granted")
		}
	}
	return err
}

func hostLabel() string {
	h, _ := os.Hostname()
	if h == "" {
		h = "unknown host"
	}
	return "shippin-bridge on " + h
}

type workspace struct {
	ID        string `json:"ID"`
	Name      string `json:"Name"`
	State     string `json:"State"`
	ProjectID string `json:"ProjectID"`
	Tenant    string `json:"Tenant"`
	TenantID  string `json:"TenantID"`
}

func workspaces(c creds) ([]workspace, string, error) {
	var me struct {
		Account    map[string]string `json:"account"`
		Workspaces []workspace       `json:"workspaces"`
	}
	err := call(c, "GET", "/v1/bridge/me", nil, &me)
	return me.Workspaces, me.Account["email"], err
}

func list() error {
	c, err := loadCreds()
	if err != nil {
		return err
	}
	ws, email, err := workspaces(c)
	if err != nil {
		return err
	}
	fmt.Printf("signed in as %s\n", email)
	for _, w := range ws {
		fmt.Printf("%s  %-24s %-10s %s\n", w.ID, w.Name, w.State, w.Tenant)
	}
	return nil
}
