// shippin-hostd is the narrowly scoped privileged helper on the Incus host.
// It listens on a unix socket exposed only to the console container via an Incus
// proxy device and performs a closed set of tenant-scoped operations.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"shippin.cloud/console/internal/enroll"
	"shippin.cloud/console/internal/healthprobe"
	"shippin.cloud/console/internal/hostd"
)

// version is set at build time (-ldflags "-X main.version=<release>").
var version = "dev"

func main() {
	path := os.Getenv("SHIPPIN_HOSTD_CONFIG")
	if path == "" {
		path = "/etc/shippin/hostd.json"
	}
	cfg, err := hostd.LoadConfig(path)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if len(os.Args) > 1 {
		if os.Args[1] != "storage" {
			log.Fatal("usage: shippin-hostd [storage show|add --tenant UUID [--quota-gib N --root-gib N --data-gib N]]")
		}
		if err := storageCmd(cfg, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	host := hostd.NewServer(cfg)
	if cfg.Enroll != nil {
		go func() {
			err := (&enroll.Agent{Cfg: *cfg.Enroll, Version: version, HandleCommand: func(ctx context.Context, command enroll.Command) (enroll.CommandResult, error) {
				if command.Type != "probe_incus_application" {
					return enroll.CommandResult{}, errors.New("unsupported host command")
				}
				var request healthprobe.ApplicationRequest
				if err := json.Unmarshal(command.Payload, &request); err != nil || request.DeploymentID != command.DeploymentID || request.Generation != command.Generation {
					return enroll.CommandResult{}, errors.New("host command payload does not match its durable target")
				}
				result := hostd.ProbeApplication(ctx, cfg.Platform, request)
				return enroll.CommandResult{Status: result.Status, DurationMS: result.DurationMS, Failure: result.Failure}, nil
			}}).Run(context.Background())
			log.Printf("enroll stopped: %v", err)
		}()
	}
	_ = os.MkdirAll(filepath.Dir(cfg.Socket), 0o755)
	_ = os.Remove(cfg.Socket)
	ln, err := net.Listen("unix", cfg.Socket)
	if err != nil {
		log.Fatal(err)
	}
	_ = os.Chmod(cfg.Socket, 0o600)
	log.Printf("shippin-hostd on %s", cfg.Socket)
	log.Fatal(http.Serve(ln, host.Routes()))
}
