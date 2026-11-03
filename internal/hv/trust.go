package hv

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TLSConfig builds verification for one hypervisor API. Verification is always on.
//
// caPEM adds anchors to the system roots. trustedPEM is one certificate an operator accepted after
// comparing its fingerprint: it becomes the only anchor, and the name checked is one the certificate
// itself carries, because a hypervisor's own certificate rarely names the address it is reached by.
func TLSConfig(caPEM, trustedPEM []byte, client *tls.Certificate) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if client != nil {
		cfg.Certificates = []tls.Certificate{*client}
	}
	if len(trustedPEM) > 0 {
		cert, err := ParseCertificate(trustedPEM)
		if err != nil {
			return nil, err
		}
		name := certificateName(cert)
		if name == "" {
			return nil, errors.New("hv: the trusted certificate names no host, so it cannot be verified")
		}
		pool := x509.NewCertPool()
		pool.AddCert(cert)
		cfg.RootCAs, cfg.ServerName = pool, name
		return cfg, nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if len(caPEM) > 0 && !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("hv: the CA file holds no usable certificate")
	}
	cfg.RootCAs = pool
	return cfg, nil
}

func certificateName(c *x509.Certificate) string {
	if len(c.DNSNames) > 0 {
		return c.DNSNames[0]
	}
	if len(c.IPAddresses) > 0 {
		return c.IPAddresses[0].String()
	}
	return ""
}

// ParseCertificate reads the first certificate in PEM.
func ParseCertificate(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("hv: not a PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("hv: certificate: %w", err)
	}
	return cert, nil
}

// PresentedCertificate is what a failed TLS handshake offered, so a person can decide whether to
// trust it. Reading it from the failure means nothing here ever turns verification off.
func PresentedCertificate(err error) *x509.Certificate {
	var failed *tls.CertificateVerificationError
	if errors.As(err, &failed) && len(failed.UnverifiedCertificates) > 0 {
		return failed.UnverifiedCertificates[0]
	}
	return nil
}

// Fingerprint is the SHA-256 of a certificate, as lowercase hex.
func Fingerprint(c *x509.Certificate) string {
	sum := sha256.Sum256(c.Raw)
	return hex.EncodeToString(sum[:])
}

// CertificateInfo is what someone needs to decide whether to trust a certificate.
type CertificateInfo struct {
	Fingerprint string    `json:"fingerprint"`
	Subject     string    `json:"subject"`
	Issuer      string    `json:"issuer"`
	Names       []string  `json:"names"`
	NotAfter    time.Time `json:"not_after"`
	PEM         string    `json:"pem"`
}

// DescribeCertificate fills CertificateInfo, with the fingerprint grouped the way hypervisors print it.
func DescribeCertificate(c *x509.Certificate) CertificateInfo {
	names := append([]string{}, c.DNSNames...)
	for _, ip := range c.IPAddresses {
		names = append(names, ip.String())
	}
	raw := Fingerprint(c)
	groups := make([]string, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		groups = append(groups, strings.ToUpper(raw[i:i+2]))
	}
	return CertificateInfo{
		Fingerprint: strings.Join(groups, ":"),
		Subject:     c.Subject.CommonName,
		Issuer:      c.Issuer.CommonName,
		Names:       names,
		NotAfter:    c.NotAfter,
		PEM:         string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})),
	}
}
