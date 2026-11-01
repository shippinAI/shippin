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
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "inspect-incus" {
		return errors.New("usage: shippin-agent inspect-incus -socket ABSOLUTE_PATH -project PROJECT")
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

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
