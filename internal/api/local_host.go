package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/hv/incus"
	"shippin.cloud/console/internal/infrastructure/servers"
)

// The machine the panel is installed on is attached by the install itself (docs/deploy.md), so the first
// page already lists it and nobody copies a token from a shell. The installer runs as root on that
// machine: it finds the Incus socket, or creates a limited Proxmox user and API token, and names them in
// the panel's environment. The console attaches them when it starts, once:
//
//	SHIPPIN_LOCAL_INCUS_SOCKET         the local Incus socket, such as /var/lib/incus/unix.socket
//	SHIPPIN_LOCAL_PROXMOX_TOKEN_ID     user@realm!token for the local Proxmox VE
//	SHIPPIN_LOCAL_PROXMOX_TOKEN_FILE   a root-only file holding that token's secret
//	SHIPPIN_LOCAL_PROXMOX_CERT_FILE    the host's certificate (/etc/pve/local/pve-ssl.pem)
//	SHIPPIN_LOCAL_PROXMOX_URL          optional, https://127.0.0.1:8006 when unset
//
// A host is saved only after it answers as a real, authenticated API, the same check Add host makes.

// Slugs of the hosts attached this way.
const (
	localIncusSlug   = "this-machine-incus"
	localProxmoxSlug = "this-machine-proxmox"
)

// localHost is one attachment the environment names, with its secret for this start only.
type localHost struct {
	ep     servers.Endpoint
	secret string
}

// localHostsFromEnv reads the attachments the environment names.
func localHostsFromEnv(getenv func(string) string, readFile func(string) ([]byte, error), machine string) ([]localHost, error) {
	var out []localHost
	if sock := getenv("SHIPPIN_LOCAL_INCUS_SOCKET"); sock != "" {
		base := "unix://" + sock
		if incus.SocketPath(base) == "" {
			return nil, fmt.Errorf("SHIPPIN_LOCAL_INCUS_SOCKET must be an absolute path")
		}
		out = append(out, localHost{ep: servers.Endpoint{Kind: "incus", BaseURL: base, Slug: localIncusSlug, Label: machine}})
	}
	if id := getenv("SHIPPIN_LOCAL_PROXMOX_TOKEN_ID"); id != "" {
		secretFile, certFile := getenv("SHIPPIN_LOCAL_PROXMOX_TOKEN_FILE"), getenv("SHIPPIN_LOCAL_PROXMOX_CERT_FILE")
		if secretFile == "" || certFile == "" {
			return nil, errors.New("SHIPPIN_LOCAL_PROXMOX_TOKEN_ID needs SHIPPIN_LOCAL_PROXMOX_TOKEN_FILE and SHIPPIN_LOCAL_PROXMOX_CERT_FILE")
		}
		secret, err := readFile(secretFile)
		if err != nil {
			return nil, fmt.Errorf("SHIPPIN_LOCAL_PROXMOX_TOKEN_FILE: %w", err)
		}
		cert, err := readFile(certFile)
		if err != nil {
			return nil, fmt.Errorf("SHIPPIN_LOCAL_PROXMOX_CERT_FILE: %w", err)
		}
		base := getenv("SHIPPIN_LOCAL_PROXMOX_URL")
		if base == "" {
			base = "https://127.0.0.1:8006"
		}
		out = append(out, localHost{secret: strings.TrimSpace(string(secret)), ep: servers.Endpoint{Kind: "proxmox", BaseURL: base,
			TokenID: id, TrustedCert: string(cert), Slug: localProxmoxSlug, Label: machine}})
	}
	return out, nil
}

// AttachLocalHosts attaches the machine the panel is installed on, as the environment names it, unless it
// is attached already. A failure is logged and the panel starts anyway; the host can be added by hand.
func (a *API) AttachLocalHosts(ctx context.Context) {
	machine, _ := os.Hostname()
	if machine == "" {
		machine = "This machine"
	}
	hosts, err := localHostsFromEnv(os.Getenv, os.ReadFile, machine)
	if err != nil {
		slog.Error("this machine was not attached", "error", err)
		return
	}
	for _, h := range hosts {
		if err := a.attachLocalHost(ctx, h); err != nil && !errors.Is(err, servers.ErrSlugTaken) {
			slog.Error("this machine was not attached", "kind", h.ep.Kind, "error", err)
		}
	}
}

func (a *API) attachLocalHost(ctx context.Context, h localHost) error {
	if a.hvDir() == nil {
		return errors.New("the host registry is not available")
	}
	if existing, err := a.hvDir().Endpoints(ctx); err == nil {
		for _, e := range existing {
			if e.Slug == h.ep.Slug {
				return servers.ErrSlugTaken
			}
		}
	}
	ep := h.ep
	ep.Site = a.uiConfig(ctx).DefaultSite
	if ep.Site == "" {
		ep.Site = ep.Slug
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	ep.ID = id.String()
	cctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := a.prepareEndpoint(cctx, &ep, hvAttach{TokenSecret: h.secret}); err != nil {
		return err
	}
	if err := a.verifyEndpoint(cctx, ep); err != nil {
		return err
	}
	if _, err := a.hvDir().CreateEndpoint(ctx, ep, "install"); err != nil {
		return err
	}
	slog.Info("this machine is attached", "kind", ep.Kind, "slug", ep.Slug)
	return nil
}
