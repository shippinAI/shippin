// Package auth implements the Nomen OIDC PKCE login (BFF) and sessions.
package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"os"
	"time"

	"shippin.cloud/console/internal/config"
)

// HTTPClient reaches the IdP via the internal edge, trusting the edge CA.
func HTTPClient(c config.IDP) *http.Client {
	pool, _ := x509.SystemCertPool()
	if pool == nil {
		pool = x509.NewCertPool()
	}
	if c.CAFile != "" {
		if pem, err := os.ReadFile(c.CAFile); err == nil {
			pool.AppendCertsFromPEM(pem)
		}
	}
	d := &net.Dialer{Timeout: 5 * time.Second}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, n, addr string) (net.Conn, error) {
			if c.Dial != "" {
				addr = c.Dial
			}
			return d.DialContext(ctx, n, addr)
		},
	}
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}
