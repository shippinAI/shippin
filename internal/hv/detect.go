package hv

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Detect works out from an address alone which kind of hypervisor answers there, so a person adding a
// server can confirm a finding instead of choosing a kind and guessing a port.
//
// It sends nothing but an unauthenticated GET to each kind's version path: no credentials, no body, and
// no redirect is followed. TLS verification stays on, as everywhere in this package. The certificate is
// read from the handshake: one that chains to a root this panel trusts completes it, and an unknown one is
// taken from the verification failure (PresentedCertificate). The GETs then go out pinned to exactly that
// certificate (TLSConfig with it as the trusted one), the same way the panel connects once a person has
// accepted it. Its answer is a suggestion for that person to confirm, never a trust decision.
//
// An address may be a host name or IP, host:port, or an https URL with neither path nor query. Without a
// port, each kind's default port is tried at once; with one, only that port, for every kind.

// DetectOutcome is how far a probe got.
type DetectOutcome string

const (
	// DetectRecognised: a kind this panel drives answered.
	DetectRecognised DetectOutcome = "recognised"
	// DetectUnrecognised: something answered, over TLS or not, but as no kind this panel drives.
	DetectUnrecognised DetectOutcome = "unrecognised"
	// DetectUnreachable: nothing accepted a connection in time.
	DetectUnreachable DetectOutcome = "unreachable"
)

func (DetectOutcome) Values() []string {
	return []string{string(DetectRecognised), string(DetectUnrecognised), string(DetectUnreachable)}
}

// Probe is what one address and port said.
type Probe struct {
	BaseURL string        `json:"base_url"`
	Outcome DetectOutcome `json:"outcome"`
	// Kind is the driver kind (hv.Specs) that answered, when one did.
	Kind    string `json:"kind,omitempty"`
	Version string `json:"version,omitempty"`
	// Certificate is what the server presented, when it spoke TLS.
	Certificate *CertificateInfo `json:"certificate"`
	// CertificateVerified says the certificate chains to a root this panel already trusts for this
	// address, so there is nothing to accept.
	CertificateVerified bool `json:"certificate_verified"`
	// Evidence is what was seen, in plain words, in the order it was seen.
	Evidence []string `json:"evidence"`
}

// Detection is the finding for an address: the best probe, and every probe made.
type Detection struct {
	// Address is the address checked, as host or host:port.
	Address string `json:"address"`
	Probe
	Probes []Probe `json:"probes"`
}

// recogniser is how one kind is told apart: the port it listens on by default, the path it answers
// without credentials, and what that answer must look like.
type recogniser struct {
	kind  string
	port  string
	path  string
	match func(res *http.Response, body []byte) (version, evidence string, ok bool)
}

// recognisers are in the order of Specs. The kinds must be ones Specs lists.
var recognisers = []recogniser{
	{kind: "incus", port: "8443", path: "/1.0", match: matchIncus},
	{kind: "proxmox", port: "8006", path: "/api2/json/version", match: matchProxmox},
}

// Probe limits. A hypervisor answers its version path at once; anything slower is not worth waiting for
// while a person watches.
const (
	detectTimeout = 8 * time.Second
	dialTimeout   = 3 * time.Second
	answerTimeout = 4 * time.Second
	maxAnswer     = 1 << 20
)

// ErrBadAddress is returned for an address Detect will not try.
var ErrBadAddress = errors.New("hv: bad address")

func badAddress(msg string) error { return fmt.Errorf("%w: %s", ErrBadAddress, msg) }

// Detect probes address. It fails only for an address it will not try (ErrBadAddress); a server that
// does not answer, or answers as nothing it knows, is a result.
func Detect(ctx context.Context, address string) (Detection, error) {
	return detect(ctx, address, recognisers)
}

func detect(ctx context.Context, address string, rs []recogniser) (Detection, error) {
	host, port, err := ParseAddress(address)
	if err != nil {
		return Detection{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, detectTimeout)
	defer cancel()
	var ports []string
	if port != "" {
		ports = []string{port}
	} else {
		for _, r := range rs {
			if !contains(ports, r.port) {
				ports = append(ports, r.port)
			}
		}
	}
	probes := make([]Probe, len(ports))
	var wg sync.WaitGroup
	for i, p := range ports {
		wg.Go(func() { probes[i] = probe(ctx, host, p, rs) })
	}
	wg.Wait()
	d := Detection{Address: host, Probes: probes}
	if port != "" {
		d.Address = net.JoinHostPort(host, port)
	}
	d.Probe = best(probes)
	for _, p := range probes {
		if p.Outcome == DetectRecognised && p.BaseURL != d.BaseURL {
			d.Evidence = append(d.Evidence, fmt.Sprintf("%s also answered, as %s; choose the kind yourself to use that one", p.BaseURL, p.Kind))
		}
	}
	return d, nil
}

// best is the first recognised probe, else the first that reached something, else the first.
func best(probes []Probe) Probe {
	for _, want := range []DetectOutcome{DetectRecognised, DetectUnrecognised} {
		for _, p := range probes {
			if p.Outcome == want {
				p.Evidence = append([]string{}, p.Evidence...)
				return p
			}
		}
	}
	p := probes[0]
	p.Evidence = append([]string{}, p.Evidence...)
	return p
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// probe connects to one port, reads its certificate, and asks each kind's version path.
func probe(ctx context.Context, host, port string, rs []recogniser) Probe {
	addr := net.JoinHostPort(host, port)
	p := Probe{BaseURL: "https://" + addr, Outcome: DetectUnreachable, Evidence: []string{}}
	say := func(f string, args ...any) { p.Evidence = append(p.Evidence, fmt.Sprintf(f, args...)) }

	conn, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		say("%s did not accept a connection: %s", addr, plainDialError(err))
		return p
	}
	p.Outcome = DetectUnrecognised
	tc := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	_ = conn.SetDeadline(time.Now().Add(dialTimeout))
	err = tc.HandshakeContext(ctx)
	var cert *x509.Certificate
	switch {
	case err == nil:
		if peers := tc.ConnectionState().PeerCertificates; len(peers) > 0 {
			cert, p.CertificateVerified = peers[0], true
		}
	default:
		cert = PresentedCertificate(err)
	}
	_ = tc.Close()
	if cert == nil {
		if err == nil {
			say("%s presented no certificate", addr)
		} else {
			say("%s accepted a connection but did not complete a TLS handshake: %s", addr, plainTLSError(err))
		}
		return p
	}
	info := DescribeCertificate(cert)
	p.Certificate = &info
	if p.CertificateVerified {
		say("its certificate is trusted by this panel for %s", host)
	} else {
		say("its certificate is not one this panel already trusts; compare the fingerprint with the server's")
	}
	cfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if !p.CertificateVerified {
		pinned, err := TLSConfig(nil, []byte(info.PEM), nil)
		if err != nil {
			say("its certificate names no host, so the panel cannot pin it and will not connect to it")
			return p
		}
		cfg = pinned
	}
	if time.Now().After(cert.NotAfter) {
		say("its certificate expired on %s, so the panel will not connect to it", cert.NotAfter.UTC().Format("2006-01-02"))
		return p
	}
	client := &http.Client{
		Timeout: answerTimeout,
		Transport: &http.Transport{
			TLSClientConfig:       cfg,
			DialContext:           (&net.Dialer{Timeout: dialTimeout}).DialContext,
			TLSHandshakeTimeout:   dialTimeout,
			ResponseHeaderTimeout: answerTimeout,
			MaxIdleConns:          1,
			IdleConnTimeout:       time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	for _, r := range rs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.BaseURL+r.path, nil)
		if err != nil {
			say("%s could not be asked: %v", r.path, err)
			continue
		}
		req.Header.Set("Accept", "application/json")
		res, err := client.Do(req)
		if err != nil {
			if PresentedCertificate(err) != nil {
				say("%s: the server presented a different certificate on the next connection", r.path)
			} else {
				say("%s did not answer: %s", r.path, plainTLSError(err))
			}
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(res.Body, maxAnswer))
		_ = res.Body.Close()
		version, evidence, ok := r.match(res, body)
		say("%s", evidence)
		if ok && p.Kind == "" {
			p.Outcome, p.Kind, p.Version = DetectRecognised, r.kind, version
		}
	}
	return p
}

func matchIncus(res *http.Response, body []byte) (string, string, bool) {
	if res.StatusCode != http.StatusOK {
		return "", fmt.Sprintf("/1.0 answered HTTP %d, which Incus does not", res.StatusCode), false
	}
	var api struct {
		Metadata struct {
			APIVersion  string `json:"api_version"`
			Auth        string `json:"auth"`
			Environment struct {
				Server        string `json:"server"`
				ServerVersion string `json:"server_version"`
			} `json:"environment"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &api) != nil || api.Metadata.APIVersion != "1.0" {
		return "", "/1.0 answered, but not with the Incus API", false
	}
	if s := api.Metadata.Environment.Server; s != "" && s != "incus" {
		return "", fmt.Sprintf("/1.0 answered as %q, which this panel does not drive", s), false
	}
	auth := api.Metadata.Auth
	if auth == "" {
		auth = "no trust state given"
	}
	return api.Metadata.Environment.ServerVersion, "answered /1.0 as Incus, " + auth, true
}

func matchProxmox(res *http.Response, body []byte) (string, string, bool) {
	pve := strings.HasPrefix(res.Header.Get("Server"), "pve-api-daemon")
	switch res.StatusCode {
	case http.StatusUnauthorized:
		if pve {
			return "", "answered /api2/json/version with 401 from pve-api-daemon: Proxmox VE, asking for a token", true
		}
		return "", "/api2/json/version asked for credentials, but not as Proxmox VE", false
	case http.StatusOK:
		var ver struct {
			Data struct {
				Version string `json:"version"`
				Release string `json:"release"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &ver) == nil && ver.Data.Version != "" && (pve || ver.Data.Release != "") {
			return ver.Data.Version, "answered /api2/json/version as Proxmox VE " + ver.Data.Version, true
		}
		return "", "/api2/json/version answered, but not as Proxmox VE", false
	}
	return "", fmt.Sprintf("/api2/json/version answered HTTP %d, which Proxmox VE does not", res.StatusCode), false
}

func plainDialError(err error) string {
	var ne net.Error
	var dns *net.DNSError
	switch {
	case errors.As(err, &dns):
		return "the name does not resolve"
	case errors.As(err, &ne) && ne.Timeout(), errors.Is(err, context.DeadlineExceeded):
		return "no answer in time"
	case strings.Contains(err.Error(), "refused"):
		return "connection refused"
	}
	return "unreachable"
}

func plainTLSError(err error) string {
	var rec tls.RecordHeaderError
	var ne net.Error
	switch {
	case errors.As(err, &rec):
		return "it does not speak TLS"
	case errors.As(err, &ne) && ne.Timeout(), errors.Is(err, context.DeadlineExceeded):
		return "no answer in time"
	case errors.Is(err, io.EOF), strings.Contains(err.Error(), "reset"):
		return "it closed the connection"
	}
	return "the connection failed"
}

var hostName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*\.?$`)

// ParseAddress reads an address a person typed: a host name or IP, host:port, or an https URL. It refuses
// other schemes, user info, a path, a query and a fragment. port is "" when none was given.
func ParseAddress(raw string) (host, port string, err error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", badAddress("enter the server's address")
	}
	if len(s) > 300 {
		return "", "", badAddress("that address is too long")
	}
	if scheme, _, ok := strings.Cut(s, "://"); ok {
		if !strings.EqualFold(scheme, "https") {
			return "", "", badAddress("only an https address can be checked")
		}
		u, err := url.Parse(s)
		if err != nil {
			return "", "", badAddress("that is not an address")
		}
		switch {
		case u.User != nil:
			return "", "", badAddress("leave the user name and password out of the address")
		case u.Path != "" && u.Path != "/", u.RawPath != "":
			return "", "", badAddress("leave the path out of the address")
		case u.RawQuery != "" || u.ForceQuery:
			return "", "", badAddress("leave the query out of the address")
		case u.Fragment != "" || strings.Contains(s, "#"):
			return "", "", badAddress("leave the fragment out of the address")
		}
		host, port = u.Hostname(), u.Port()
		if strings.HasSuffix(u.Host, ":") {
			return "", "", badAddress("the port is missing after the colon")
		}
	} else {
		if strings.ContainsAny(s, "/?#@ \t") {
			return "", "", badAddress("enter a host name, host:port or an https address")
		}
		switch h, p, err := net.SplitHostPort(s); {
		case err == nil:
			host, port = h, p
			if port == "" {
				return "", "", badAddress("the port is missing after the colon")
			}
		case strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]"):
			host = s[1 : len(s)-1]
		case strings.Count(s, ":") > 1:
			host = s // a bare IPv6 address
		case strings.Contains(s, ":"):
			return "", "", badAddress("that port is not a number")
		default:
			host = s
		}
	}
	if ip := net.ParseIP(host); ip == nil && (strings.Contains(host, ":") || len(host) > 253 || !hostName.MatchString(host)) {
		return "", "", badAddress("that host name is not valid")
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
			return "", "", badAddress("the port must be a number from 1 to 65535")
		}
	}
	return host, port, nil
}
