// Package acquisition imports explicitly configured acquisition-source pages.
// It is a private integration, not a waitlist verification or customer-creation path.
package acquisition

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const pageSize = 100
const maxResponseBytes = 1 << 20

var leadIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var scopePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

var (
	ErrInvalidConfig = errors.New("invalid acquisition source configuration")
	ErrUnavailable   = errors.New("acquisition source is unavailable")
	ErrConflict      = errors.New("acquisition source changed concurrently")
	ErrNotFound      = errors.New("acquisition source not found")
)

// SourceConfig contains the nonsecret values required for one source request.
// SigningKey is supplied separately after opening its source-bound ciphertext.
type SourceConfig struct {
	ID, Origin, DialIP, SourceTenant, SourceProject, Issuer, Audience, CAPEM string
	Port                                                                     int
}

// Lead is the source's version-one export record. Email remains personal data,
// and Deleted records must not carry it or any other payload fields.
type Lead struct {
	ID             string    `json:"id"`
	Email          string    `json:"email,omitempty"`
	EmailVerified  bool      `json:"-"`
	Status         string    `json:"status,omitempty"`
	ConsentVersion string    `json:"consent_version,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
	ConsentAt      time.Time `json:"consent_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	Revision       uint64    `json:"revision"`
	Deleted        bool      `json:"deleted,omitempty"`
}

type Page struct {
	Items         []Lead
	Cursor        uint64
	RetentionDays int
	More          bool
}

// Fetcher fetches at most one page. Implementations must never expose or retain the key.
type Fetcher interface {
	Fetch(context.Context, SourceConfig, []byte, uint64) (Page, error)
}

type HTTPFetcher struct{}

// Fetch performs one bounded, pinned-IP HTTPS request and mints a short-lived,
// source-scoped development delegation token in process.
func (HTTPFetcher) Fetch(parent context.Context, source SourceConfig, seed []byte, after uint64) (Page, error) {
	u, ip, roots, err := validateSource(source)
	if err != nil || len(seed) != ed25519.SeedSize || after > uint64(^uint64(0)>>1) {
		return Page{}, ErrInvalidConfig
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	private := ed25519.NewKeyFromSeed(seed)
	defer clear(private)
	token, err := delegationToken(private, source, time.Now().UTC())
	if err != nil {
		return Page{}, ErrUnavailable
	}
	transport := &http.Transport{
		Proxy:                  nil,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname(), RootCAs: roots},
		MaxResponseHeaderBytes: 64 << 10,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(source.Port)))
		},
		DisableKeepAlives: true,
	}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer transport.CloseIdleConnections()
	endpoint := *u
	endpoint.Path = "/v1/projects/" + source.SourceProject + "/marketing/leads"
	endpoint.RawQuery = "after=" + strconv.FormatUint(after, 10) + "&limit=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Page{}, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return Page{}, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Page{}, ErrUnavailable
	}
	mediaType, _, mediaErr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		return Page{}, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(raw) > maxResponseBytes {
		return Page{}, ErrUnavailable
	}
	page, err := decodePage(raw, after)
	if err != nil {
		return Page{}, ErrUnavailable
	}
	return page, nil
}

func validateSource(s SourceConfig) (*url.URL, net.IP, *x509.CertPool, error) {
	parsedID, idErr := uuid.Parse(s.ID)
	if idErr != nil || parsedID.String() != s.ID || !scopePattern.MatchString(s.SourceTenant) || !scopePattern.MatchString(s.SourceProject) ||
		strings.TrimSpace(s.Issuer) != s.Issuer || s.Issuer == "" || len(s.Issuer) > 512 || strings.ContainsAny(s.Issuer, "\r\n\x00") ||
		strings.TrimSpace(s.Audience) != s.Audience || s.Audience == "" || len(s.Audience) > 512 || strings.ContainsAny(s.Audience, "\r\n\x00") || s.Port < 1 || s.Port > 65535 {
		return nil, nil, nil, ErrInvalidConfig
	}
	u, err := url.Parse(s.Origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" || u.Hostname() == "" || strings.ContainsAny(s.Origin, "\r\n\x00") {
		return nil, nil, nil, ErrInvalidConfig
	}
	if rawPort := u.Port(); rawPort != "" {
		configuredPort, err := strconv.Atoi(rawPort)
		if err != nil || configuredPort != s.Port {
			return nil, nil, nil, ErrInvalidConfig
		}
	}
	if strings.EqualFold(u.Hostname(), "metadata") || strings.EqualFold(u.Hostname(), "metadata.google.internal") {
		return nil, nil, nil, ErrInvalidConfig
	}
	ip := net.ParseIP(s.DialIP)
	if ip == nil || (!ip.IsLoopback() && !ip.IsGlobalUnicast()) || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || isCloudMetadataIP(ip) {
		return nil, nil, nil, ErrInvalidConfig
	}
	if strings.TrimSpace(s.CAPEM) == "" || len(s.CAPEM) > 64*1024 {
		return nil, nil, nil, ErrInvalidConfig
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(s.CAPEM)) {
		return nil, nil, nil, ErrInvalidConfig
	}
	return u, ip, roots, nil
}

func isCloudMetadataIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		return v4.Equal(net.IPv4(100, 100, 100, 200)) || v4.Equal(net.IPv4(168, 63, 129, 16))
	}
	return ip.Equal(net.ParseIP("fd00:ec2::254"))
}

type sourceClaims struct {
	Issuer       string   `json:"iss"`
	Audience     string   `json:"aud"`
	Subject      string   `json:"sub"`
	Tenant       string   `json:"tenant"`
	Projects     []string `json:"projects"`
	Repositories []string `json:"repositories"`
	Profiles     []string `json:"profiles"`
	Scopes       []string `json:"scopes"`
	Expiry       int64    `json:"exp"`
	NotBefore    int64    `json:"nbf"`
}

func delegationToken(key ed25519.PrivateKey, source SourceConfig, now time.Time) (string, error) {
	if len(key) != ed25519.PrivateKeySize {
		return "", ErrInvalidConfig
	}
	header, err := json.Marshal(struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}{"EdDSA", "JWT"})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(sourceClaims{Issuer: source.Issuer, Audience: source.Audience, Subject: "acquisition:" + source.ID,
		Tenant: source.SourceTenant, Projects: []string{source.SourceProject}, Repositories: []string{}, Profiles: []string{}, Scopes: []string{"marketing:read"},
		Expiry: now.Add(120 * time.Second).Unix(), NotBefore: now.Unix()})
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding.EncodeToString
	input := enc(header) + "." + enc(claims)
	return input + "." + enc(ed25519.Sign(key, []byte(input))), nil
}

type envelope struct {
	Version       int    `json:"version"`
	Items         []Lead `json:"items"`
	NextCursor    uint64 `json:"next_cursor"`
	RetentionDays int    `json:"retention_days"`
}

func decodePage(raw []byte, after uint64) (Page, error) {
	rootFields, err := strictObjectFields(raw, map[string]bool{"version": true, "items": true, "next_cursor": true, "retention_days": true})
	if err != nil || len(rootFields) != 4 {
		return Page{}, fmt.Errorf("invalid source page")
	}
	var e envelope
	if json.Unmarshal(raw, &e) != nil || e.Version != 1 || e.RetentionDays < 1 || e.RetentionDays > 730 || len(e.Items) > pageSize {
		return Page{}, fmt.Errorf("invalid source page")
	}
	var itemRaw []json.RawMessage
	itemsJSON := bytes.TrimSpace(rootFields["items"])
	if len(itemsJSON) == 0 || itemsJSON[0] != '[' || json.Unmarshal(itemsJSON, &itemRaw) != nil || len(itemRaw) != len(e.Items) {
		return Page{}, fmt.Errorf("invalid source page")
	}
	allowedLeadFields := map[string]bool{"id": true, "email": true, "status": true, "consent_version": true, "created_at": true, "consent_at": true, "updated_at": true, "expires_at": true, "revision": true, "deleted": true}
	last := after
	for i := range e.Items {
		lead := &e.Items[i]
		if _, err := strictObjectFields(itemRaw[i], allowedLeadFields); err != nil {
			return Page{}, fmt.Errorf("invalid source record")
		}
		if !leadIDPattern.MatchString(lead.ID) || lead.Revision <= last || lead.Revision > uint64(^uint64(0)>>1) {
			return Page{}, fmt.Errorf("invalid source revision")
		}
		if lead.Deleted {
			if lead.Email != "" || lead.Status != "" || lead.ConsentVersion != "" || !lead.CreatedAt.IsZero() || !lead.ConsentAt.IsZero() || !lead.UpdatedAt.IsZero() || !lead.ExpiresAt.IsZero() || lead.EmailVerified {
				return Page{}, fmt.Errorf("source tombstone carries data")
			}
		} else {
			future := time.Now().UTC().Add(5 * time.Minute)
			parsedEmail, emailErr := mail.ParseAddress(lead.Email)
			if emailErr != nil || parsedEmail.Address != lead.Email || parsedEmail.Name != "" || len(lead.Email) > 254 || strings.TrimSpace(lead.Email) != lead.Email || strings.ContainsAny(lead.Email, "\r\n\x00") ||
				(lead.Status != "new" && lead.Status != "contacted" && lead.Status != "archived") || lead.ConsentVersion != "early-access-v1" || lead.CreatedAt.IsZero() || lead.ConsentAt.IsZero() || lead.UpdatedAt.IsZero() || lead.ExpiresAt.IsZero() || !lead.ExpiresAt.After(lead.CreatedAt) || lead.ConsentAt.Before(lead.CreatedAt) || lead.UpdatedAt.Before(lead.ConsentAt) || lead.CreatedAt.After(future) || lead.ConsentAt.After(future) || lead.UpdatedAt.After(future) || lead.ExpiresAt.After(lead.ConsentAt.AddDate(0, 0, e.RetentionDays)) {
				return Page{}, fmt.Errorf("invalid source lead")
			}
			lead.EmailVerified = false
		}
		last = lead.Revision
	}
	if e.NextCursor != last {
		return Page{}, fmt.Errorf("invalid source cursor")
	}
	return Page{Items: e.Items, Cursor: e.NextCursor, RetentionDays: e.RetentionDays, More: len(e.Items) == pageSize}, nil
}

// strictObjectFields validates one JSON object without first collapsing it into
// a map, so duplicate, unknown and case-variant keys cannot override fields.
func strictObjectFields(raw []byte, allowed map[string]bool) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, ErrUnavailable
	}
	fields := make(map[string]json.RawMessage, len(allowed))
	for d.More() {
		tok, err = d.Token()
		name, ok := tok.(string)
		if err != nil || !ok || !allowed[name] {
			return nil, ErrUnavailable
		}
		if _, exists := fields[name]; exists {
			return nil, ErrUnavailable
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil, ErrUnavailable
		}
		fields[name] = value
	}
	if tok, err = d.Token(); err != nil || tok != json.Delim('}') {
		return nil, ErrUnavailable
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, ErrUnavailable
	}
	return fields, nil
}
