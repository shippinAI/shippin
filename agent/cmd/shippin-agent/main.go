// shippin-agent currently provides local, read-only provider diagnostics.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"shippin.cloud/agent/internal/incus"
	"shippin.cloud/agent/internal/proxmox"
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: shippin-agent inspect-incus | inspect-proxmox [options]")
	}
	if args[0] == "inspect-proxmox" {
		return inspectProxmox(ctx, args[1:], stdout, stderr)
	}
	if args[0] != "inspect-incus" {
		return errors.New("unknown command")
	}
	flags := flag.NewFlagSet("inspect-incus", flag.ContinueOnError)
	flags.SetOutput(stderr)
	socket := flags.String("socket", "", "absolute path to the local Incus Unix socket")
	project := flags.String("project", "", "Incus project to inspect")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional argument")
	}
	client, err := incus.NewLocal(*socket)
	if err != nil {
		return err
	}
	observation, err := client.Inspect(ctx, *project)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(observation)
}

func inspectProxmox(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("inspect-proxmox", flag.ContinueOnError)
	flags.SetOutput(stderr)
	endpoint := flags.String("endpoint", "", "Proxmox HTTPS origin")
	caFile := flags.String("ca-file", "", "optional PEM CA certificate file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional argument")
	}
	var ca []byte
	if *caFile != "" {
		var err error
		ca, err = os.ReadFile(*caFile)
		if err != nil || len(ca) == 0 {
			return errors.New("cannot read proxmox CA certificate")
		}
	}
	client, err := proxmox.New(*endpoint, os.Getenv("SHIPPIN_PROXMOX_TOKEN_ID"), os.Getenv("SHIPPIN_PROXMOX_TOKEN_SECRET"), ca)
	if err != nil {
		return err
	}
	observation, err := client.Inspect(ctx)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(observation)
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
