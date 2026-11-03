// Package tlsfile serves a certificate and key from files and reloads them when they change, so a
// renewed certificate takes effect without restarting the panel.
package tlsfile

import (
	"crypto/tls"
	"os"
	"sync"
	"time"
)

// Loader keeps the last good key pair and reloads when either file's modification time changes.
type Loader struct {
	CertFile, KeyFile string

	mu      sync.Mutex
	cert    *tls.Certificate
	certMod time.Time
	keyMod  time.Time
}

// New loads the pair once so a bad path fails at startup instead of on the first handshake.
func New(certFile, keyFile string) (*Loader, error) {
	l := &Loader{CertFile: certFile, KeyFile: keyFile}
	if _, err := l.get(); err != nil {
		return nil, err
	}
	return l, nil
}

// GetCertificate is the tls.Config hook.
func (l *Loader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) { return l.get() }

// Config is a server TLS config that uses the loader.
func (l *Loader) Config() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: l.GetCertificate}
}

func (l *Loader) get() (*tls.Certificate, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cs, cerr := os.Stat(l.CertFile)
	ks, kerr := os.Stat(l.KeyFile)
	if l.cert != nil && cerr == nil && kerr == nil && cs.ModTime().Equal(l.certMod) && ks.ModTime().Equal(l.keyMod) {
		return l.cert, nil
	}
	if cerr != nil || kerr != nil {
		if l.cert != nil {
			return l.cert, nil
		}
		if cerr != nil {
			return nil, cerr
		}
		return nil, kerr
	}
	c, err := tls.LoadX509KeyPair(l.CertFile, l.KeyFile)
	if err != nil {
		if l.cert != nil {
			return l.cert, nil // a half-written renewal keeps the previous pair
		}
		return nil, err
	}
	l.cert, l.certMod, l.keyMod = &c, cs.ModTime(), ks.ModTime()
	return l.cert, nil
}
