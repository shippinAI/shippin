package hostd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// run executes a fixed binary (incus, zfs, zpool, sops) with argv, never via a shell.
func run(ctx context.Context, stdin string, bin string, args ...string) (string, error) {
	return runFor(ctx, 5*time.Minute, stdin, bin, args...)
}

// runFor is run with an explicit cap. Desktop prerequisite apt can outlive the default five minutes.
func runFor(ctx context.Context, timeout time.Duration, stdin string, bin string, args ...string) (string, error) {
	switch bin {
	case "incus", "zfs", "zpool", "sops":
	default:
		return "", fmt.Errorf("binary not allowed")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%s %s: %s", bin, args[0], strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// runner is the exec seam: tests swap it for a fake that records argv instead of running incus.
var runner = run

func incus(ctx context.Context, args ...string) (string, error) {
	return runner(ctx, "", "incus", args...)
}

// query GETs an Incus API path and decodes the metadata.
func query(ctx context.Context, path string, out any) error {
	s, err := incus(ctx, "query", path)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(s), out)
}

func exists(ctx context.Context, args ...string) bool {
	_, err := incus(ctx, args...)
	return err == nil
}

// ignoreExists treats "already exists" as success for idempotent steps.
func ignoreExists(_ string, err error) error {
	if err != nil && strings.Contains(err.Error(), "already exists") {
		return nil
	}
	return err
}
