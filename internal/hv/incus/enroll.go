package incus

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"

	"shippin.cloud/console/internal/hv"
)

// TrustToken is what `incus config trust add <name>` prints: where the server is, the certificate it
// will present, and a one-time secret.
type TrustToken struct {
	ClientName  string   `json:"client_name"`
	Fingerprint string   `json:"fingerprint"`
	Addresses   []string `json:"addresses"`
	Secret      string   `json:"secret"`
}

// ParseTrustToken decodes a token. It carries a secret, so errors never quote it.
func ParseTrustToken(s string) (TrustToken, error) {
	s = strings.TrimSpace(s)
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		if raw, err = base64.RawURLEncoding.DecodeString(s); err != nil {
			return TrustToken{}, errors.New("incus: that is not a trust token; run incus config trust add and paste what it prints")
		}
	}
	var t TrustToken
	if json.Unmarshal(raw, &t) != nil || t.Fingerprint == "" || len(t.Addresses) == 0 || t.Secret == "" {
		return TrustToken{}, errors.New("incus: that is not a trust token; run incus config trust add and paste what it prints")
	}
	return t, nil
}

// Reachable is the first address in a token whose certificate matches it.
type Reachable struct {
	BaseURL string
	Cert    *x509.Certificate
}

// Locate tries each address in the token. Nothing secret is sent. A certificate that does not match
// the token is refused outright: it means this is not the server the token came from.
func Locate(ctx context.Context, t TrustToken) (Reachable, error) {
	var last error
	for _, addr := range t.Addresses {
		cert, err := presented(ctx, addr)
		if err != nil {
			last = err
			continue
		}
		if !strings.EqualFold(hv.Fingerprint(cert), t.Fingerprint) {
			return Reachable{}, fmt.Errorf("incus: %s presented a certificate that does not match the token, so nothing was sent to it", addr)
		}
		return Reachable{BaseURL: "https://" + addr, Cert: cert}, nil
	}
	if last == nil {
		last = errors.New("no addresses")
	}
	return Reachable{}, fmt.Errorf("incus: none of the token's addresses (%s) answered from this panel. Check core.https_address and the route: %w", strings.Join(t.Addresses, ", "), last)
}

// presented reads the server's certificate from the handshake's own verification failure, so
// verification stays on. A publicly trusted certificate simply completes the handshake.
func presented(ctx context.Context, addr string) (*x509.Certificate, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("incus: address %q: %w", addr, err)
	}
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		if cert := hv.PresentedCertificate(err); cert != nil {
			return cert, nil
		}
		return nil, err
	}
	defer conn.Close()
	state := conn.(*tls.Conn).ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return nil, errors.New("incus: the server presented no certificate")
	}
	return state.PeerCertificates[0], nil
}

// Enrollment is what the panel keeps after a token is accepted. KeyPEM is secret; seal it.
type Enrollment struct {
	BaseURL    string
	TrustedPEM []byte
	CertPEM    []byte
	KeyPEM     []byte
	Version    string
}

// Enroll spends a one-time token: it makes a client certificate for this host and asks the server to
// trust it, then proves the server does before reporting success.
func Enroll(ctx context.Context, tokenText, project string) (Enrollment, error) {
	token, err := ParseTrustToken(tokenText)
	if err != nil {
		return Enrollment{}, err
	}
	found, err := Locate(ctx, token)
	if err != nil {
		return Enrollment{}, err
	}
	certPEM, keyPEM, cert, err := newClientCertificate()
	if err != nil {
		return Enrollment{}, err
	}
	trusted := hv.DescribeCertificate(found.Cert).PEM
	c, err := New(Endpoint{BaseURL: found.BaseURL, Project: project, TrustedPEM: []byte(trusted), ClientCert: &cert}, func() (string, error) { return "", nil })
	if err != nil {
		return Enrollment{}, err
	}
	// trust_token is the current field; password is the same value for servers from before the rename.
	raw, _ := json.Marshal(map[string]string{"type": "client", "name": token.ClientName, "trust_token": strings.TrimSpace(tokenText), "password": strings.TrimSpace(tokenText)})
	body, code, err := c.do(ctx, http.MethodPost, "/1.0/certificates", raw)
	if err != nil {
		return Enrollment{}, fmt.Errorf("incus: sending the token: %w", err)
	}
	if code < 200 || code >= 300 {
		return Enrollment{}, fmt.Errorf("incus: the server refused the token (it works once and expires): HTTP %d: %s", code, clip(body))
	}
	health, err := c.Health(ctx)
	if err != nil {
		return Enrollment{}, fmt.Errorf("incus: the token was accepted but the panel still cannot read the server: %w", err)
	}
	if err := c.requireTrusted(ctx); err != nil {
		return Enrollment{}, err
	}
	return Enrollment{BaseURL: found.BaseURL, TrustedPEM: []byte(trusted), CertPEM: certPEM, KeyPEM: keyPEM, Version: health.Version}, nil
}

func (c *Client) requireTrusted(ctx context.Context) error {
	body, code, err := c.do(ctx, http.MethodGet, "/1.0", nil)
	if err != nil {
		return err
	}
	var r struct {
		Metadata struct {
			Auth string `json:"auth"`
		} `json:"metadata"`
	}
	if code != http.StatusOK || json.Unmarshal(body, &r) != nil || r.Metadata.Auth != "trusted" {
		return errors.New("incus: the server does not report this panel as trusted")
	}
	return nil
}

func newClientCertificate() (certPEM, keyPEM []byte, cert tls.Certificate, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, nil, tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, tls.Certificate{}, err
	}
	tpl := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "shippin-panel"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(10, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, tls.Certificate{}, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err = tls.X509KeyPair(certPEM, keyPEM)
	return certPEM, keyPEM, cert, err
}

// Details reads what the server says about itself, and its cluster when it is in one.
func (c *Client) Details(ctx context.Context) (hv.NodeDetails, error) {
	body, code, err := c.do(ctx, http.MethodGet, c.path("/1.0"), nil)
	if err != nil {
		return hv.NodeDetails{}, err
	}
	if code != http.StatusOK {
		return hv.NodeDetails{}, fmt.Errorf("incus: server HTTP %d: %s", code, clip(body))
	}
	var srv struct {
		Metadata struct {
			Environment struct {
				ServerName      string `json:"server_name"`
				ServerClustered bool   `json:"server_clustered"`
				KernelVersion   string `json:"kernel_version"`
				OSName          string `json:"os_name"`
				OSVersion       string `json:"os_version"`
			} `json:"environment"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &srv); err != nil {
		return hv.NodeDetails{}, err
	}
	env := srv.Metadata.Environment
	d := hv.NodeDetails{Hostname: env.ServerName, Kernel: env.KernelVersion, OS: strings.TrimSpace(env.OSName + " " + env.OSVersion)}
	c.addResources(ctx, &d)
	if env.ServerClustered {
		c.addMembers(ctx, &d, env.ServerName)
	}
	return d, nil
}

func (c *Client) addResources(ctx context.Context, d *hv.NodeDetails) {
	body, code, err := c.do(ctx, http.MethodGet, "/1.0/resources", nil)
	if err != nil || code != http.StatusOK {
		return
	}
	var r struct {
		Metadata struct {
			CPU struct {
				Total   int `json:"total"`
				Sockets []struct {
					Name string `json:"name"`
				} `json:"sockets"`
			} `json:"cpu"`
			Memory struct {
				Total int64 `json:"total"`
				Used  int64 `json:"used"`
			} `json:"memory"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &r) != nil {
		return
	}
	d.CPUs, d.MemTotal, d.MemUsed = r.Metadata.CPU.Total, r.Metadata.Memory.Total, r.Metadata.Memory.Used
	if len(r.Metadata.CPU.Sockets) > 0 {
		d.CPUModel = r.Metadata.CPU.Sockets[0].Name
	}
}

func (c *Client) addMembers(ctx context.Context, d *hv.NodeDetails, self string) {
	body, code, err := c.do(ctx, http.MethodGet, "/1.0/cluster/members?recursion=1", nil)
	if err != nil || code != http.StatusOK {
		return
	}
	var r struct {
		Metadata []struct {
			ServerName string `json:"server_name"`
			Status     string `json:"status"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &r) != nil {
		return
	}
	for _, m := range r.Metadata {
		d.Members = append(d.Members, hv.ClusterMember{Name: m.ServerName, Online: strings.EqualFold(m.Status, "online"), Local: m.ServerName == self})
	}
}
