package api

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/hv/incus"
	"shippin.cloud/console/internal/servers"
)

// connectTimeout bounds a connection test and an enrollment, so a dead address answers quickly.
const connectTimeout = 15 * time.Second

func (a *API) sealer() (*servers.Sealer, error) { return servers.NewSealer(a.cfg.SessionKey) }

// secretFor resolves a host's API secret at call time, so it never sits in a driver's configuration.
// A secret typed in the panel is sealed in the database; a token_env names a variable instead.
func (a *API) secretFor(ep servers.Endpoint) func() (string, error) {
	return func() (string, error) {
		if ep.Secret != "" {
			return ep.Secret, nil
		}
		if len(ep.SealedSecret) > 0 {
			s, err := a.sealer()
			if err != nil {
				return "", err
			}
			plain, err := s.Open(ep.SealedSecret, ep.ID)
			if err != nil {
				return "", err
			}
			return string(plain), nil
		}
		if ep.TokenEnv != "" {
			if v := os.Getenv(ep.TokenEnv); v != "" {
				return v, nil
			}
			return "", fmt.Errorf("token env %s is empty", ep.TokenEnv)
		}
		return "", nil
	}
}

func (a *API) clientCertificate(ep servers.Endpoint) (*tls.Certificate, error) {
	switch {
	case ep.ClientCert != "" && len(ep.SealedKey) > 0:
		s, err := a.sealer()
		if err != nil {
			return nil, err
		}
		key, err := s.Open(ep.SealedKey, ep.ID)
		if err != nil {
			return nil, err
		}
		cert, err := tls.X509KeyPair([]byte(ep.ClientCert), key)
		if err != nil {
			return nil, fmt.Errorf("incus: stored client certificate: %w", err)
		}
		return &cert, nil
	case ep.CertFile != "":
		cert, err := tls.LoadX509KeyPair(ep.CertFile, ep.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("incus: reading client certificate: %w", err)
		}
		return &cert, nil
	}
	return nil, nil
}

type connectResult struct {
	OK          bool                `json:"ok"`
	Version     string              `json:"version,omitempty"`
	Detail      string              `json:"detail,omitempty"`
	Error       string              `json:"error,omitempty"`
	Nodes       []string            `json:"nodes,omitempty"`
	Certificate *hv.CertificateInfo `json:"certificate,omitempty"`
}

// infraTest tries what the add form holds and stores nothing. A one-time token is checked without
// being spent, so testing it does not use it up.
func (a *API) infraTest(w http.ResponseWriter, r *http.Request, _ *caller) {
	var in hvAttach
	if !decodeStrict(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), connectTimeout)
	defer cancel()
	if in.TrustToken != "" {
		writeJSON(w, testTrustToken(ctx, in.TrustToken))
		return
	}
	ep := in.endpoint()
	ep.Secret = in.TokenSecret
	if err := validateEndpoint(ep, in); err != nil {
		writeJSON(w, connectResult{Error: err.Error()})
		return
	}
	d, err := a.openDriver(ep)
	if err != nil {
		writeJSON(w, connectResult{Error: err.Error()})
		return
	}
	h, err := d.Health(ctx)
	if err != nil {
		out := connectResult{Error: err.Error()}
		if cert := hv.PresentedCertificate(err); cert != nil {
			info := hv.DescribeCertificate(cert)
			out.Certificate = &info
			out.Error = "This server's certificate is not trusted yet."
		}
		writeJSON(w, out)
		return
	}
	out := connectResult{OK: h.Online, Version: h.Version}
	if lister, ok := d.(hv.NodeLister); ok {
		if nodes, err := lister.Nodes(ctx); err == nil {
			out.Nodes = nodes
		}
	}
	writeJSON(w, out)
}

func testTrustToken(ctx context.Context, tokenText string) connectResult {
	token, err := incus.ParseTrustToken(tokenText)
	if err != nil {
		return connectResult{Error: err.Error()}
	}
	found, err := incus.Locate(ctx, token)
	if err != nil {
		return connectResult{Error: err.Error()}
	}
	return connectResult{OK: true, Detail: found.BaseURL + " answered, and its certificate matches the token."}
}

// prepareEndpoint does what needs the outside world, after the caller is known to be allowed: it
// spends a trust token, seals secrets, and finds a single node. The form's plain secret stops here.
func (a *API) prepareEndpoint(ctx context.Context, ep *servers.Endpoint, in hvAttach) error {
	if ep.ID == "" {
		return errors.New("a host needs an id before its secrets can be sealed")
	}
	if in.TrustToken != "" {
		enrolled, err := incus.Enroll(ctx, in.TrustToken, ep.Node)
		if err != nil {
			return err
		}
		s, err := a.sealer()
		if err != nil {
			return err
		}
		box, err := s.Seal(enrolled.KeyPEM, ep.ID)
		if err != nil {
			return err
		}
		ep.BaseURL, ep.TrustedCert, ep.ClientCert, ep.SealedKey = enrolled.BaseURL, string(enrolled.TrustedPEM), string(enrolled.CertPEM), box
	}
	if in.TokenSecret != "" {
		s, err := a.sealer()
		if err != nil {
			return err
		}
		box, err := s.Seal([]byte(in.TokenSecret), ep.ID)
		if err != nil {
			return err
		}
		ep.SealedSecret = box
		ep.Secret = in.TokenSecret
	}
	if ep.Kind == "proxmox" && ep.Node == "" {
		d, err := a.openDriver(*ep)
		if err != nil {
			return err
		}
		if lister, ok := d.(hv.NodeLister); ok {
			nodes, err := lister.Nodes(ctx)
			if err != nil {
				return fmt.Errorf("could not list its nodes, so name the node: %w", err)
			}
			if len(nodes) != 1 {
				return fmt.Errorf("this cluster has %d nodes; name the one to reach it through: %s", len(nodes), strings.Join(nodes, ", "))
			}
			ep.Node = nodes[0]
		}
	}
	ep.Secret = ""
	return nil
}

// selfGuest is the guest this panel runs on, if it can tell. Stopping it takes the panel down, so
// nothing does that without being told to. A site sets SHIPPIN_SELF_GUEST when names differ.
func selfGuest() string {
	if v := strings.TrimSpace(os.Getenv("SHIPPIN_SELF_GUEST")); v != "" {
		return v
	}
	h, _ := os.Hostname()
	return h
}

func isSelf(g hv.Guest) bool {
	self := selfGuest()
	return self != "" && (strings.EqualFold(g.Name, self) || strings.EqualFold(g.ID, self))
}

func markSelf(items []hv.Guest) []hv.Guest {
	for i := range items {
		items[i].Self = isSelf(items[i])
	}
	return items
}

// refuseSelf stops a stop, shutdown or reboot of the panel's own guest unless the caller confirmed it.
func refuseSelf(w http.ResponseWriter, r *http.Request, d hv.Driver, guest, verb string) bool {
	if verb == "start" || r.URL.Query().Get("confirm") == "self" {
		return false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	g, err := d.Guest(ctx, guest)
	if err != nil || !isSelf(g) {
		return false
	}
	writeHVProblem(w, http.StatusConflict, "self_guest", "This panel runs on that guest. Doing this takes the panel down, and something outside it must bring it back.", "", "")
	return true
}

func runVerb(ctx context.Context, d hv.Driver, verb, guest string) (hv.Guest, error) {
	switch verb {
	case "start":
		return d.Start(ctx, guest)
	case "stop":
		return d.Stop(ctx, guest)
	case "shutdown":
		return d.Shutdown(ctx, guest)
	}
	return d.Reboot(ctx, guest)
}
