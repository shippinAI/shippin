package mail

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a tiny submission server: implicit TLS or STARTTLS, AUTH PLAIN, one message
// per transaction. rcptCode lets a test force temporary (4xx) or permanent (5xx) refusals.
type fakeSMTP struct {
	addr, caFile, user, pass string
	startTLS                 bool
	mu                       sync.Mutex
	rcptCode                 string
	got                      []string // raw DATA
	authOK                   int
}

func newFakeSMTP(t *testing.T, startTLS bool) *fakeSMTP {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "mail.test"}, DNSNames: []string{"mail.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{user: "notices@test", pass: "pw-test", startTLS: startTLS, rcptCode: "250 OK",
		caFile: filepath.Join(t.TempDir(), "ca.pem")}
	_ = os.WriteFile(f.caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	tc := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	var ln net.Listener
	if startTLS {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	} else {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", tc)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	f.addr = ln.Addr().String()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c, tc)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(c net.Conn, tc *tls.Config) {
	defer c.Close()
	r, w := bufio.NewReader(c), c
	say := func(s string) { w.Write([]byte(s + "\r\n")) }
	say("220 mail.test ESMTP")
	tlsOn := !f.startTLS
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			if !tlsOn {
				say("250-mail.test\r\n250 STARTTLS")
			} else {
				say("250-mail.test\r\n250 AUTH PLAIN")
			}
		case cmd == "STARTTLS":
			say("220 go ahead")
			tlsConn := tls.Server(c, tc)
			c, r, w, tlsOn = tlsConn, bufio.NewReader(tlsConn), tlsConn, true
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line)[len("AUTH PLAIN "):])
			if string(raw) == "\x00"+f.user+"\x00"+f.pass && tlsOn {
				f.mu.Lock()
				f.authOK++
				f.mu.Unlock()
				say("235 ok")
			} else {
				say("535 bad credentials")
			}
		case strings.HasPrefix(cmd, "MAIL FROM"):
			say("250 OK")
		case strings.HasPrefix(cmd, "RCPT TO"):
			f.mu.Lock()
			say(f.rcptCode)
			f.mu.Unlock()
		case cmd == "DATA":
			say("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.got = append(f.got, b.String())
			f.mu.Unlock()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("502 unknown")
		}
	}
}

func (f *fakeSMTP) messages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.got...)
}
