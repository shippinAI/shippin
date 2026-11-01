package main

// A fake Shippin console (bridge API and relay, over TLS) and a fake devspace sshd that trusts
// only certificates signed by the console's CA for its own principal.

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type world struct {
	url, caFile string
	certs       atomic.Int32
}

func newWorld(t *testing.T) *world {
	_, caKey, _ := ed25519.GenerateKey(rand.Reader)
	ca, _ := ssh.NewSignerFromKey(caKey)
	sshd := startSSHD(t, ca.PublicKey())
	w := &world{}
	mux := http.NewServeMux()
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(rw http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				http.Error(rw, "no", http.StatusUnauthorized)
				return
			}
			h(rw, r)
		}
	}
	mux.HandleFunc("GET /v1/bridge/me", authed(func(rw http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(rw).Encode(map[string]any{"account": map[string]string{"email": "t@example.test"},
			"workspaces": []workspace{{ID: "w1", Name: "dev"}}})
	}))
	mux.HandleFunc("POST /v1/bridge/workspaces/w1/cert", authed(func(rw http.ResponseWriter, r *http.Request) {
		var in struct {
			PublicKey string `json:"public_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(in.PublicKey))
		if err != nil {
			http.Error(rw, "bad key", http.StatusBadRequest)
			return
		}
		now := uint64(time.Now().Unix())
		cert := &ssh.Certificate{Key: pub, CertType: ssh.UserCert, KeyId: "test", ValidPrincipals: []string{"ws-w1"},
			ValidAfter: now - 60, ValidBefore: now + 600}
		_ = cert.SignCert(rand.Reader, ca)
		w.certs.Add(1)
		_ = json.NewEncoder(rw).Encode(map[string]string{"certificate": string(ssh.MarshalAuthorizedKey(cert)), "ssh_user": "dev"})
	}))
	mux.HandleFunc("GET /v1/bridge/workspaces/w1/relay", authed(func(rw http.ResponseWriter, _ *http.Request) {
		conn, brw, err := rw.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		up, err := net.Dial("tcp", sshd)
		if err != nil {
			conn.Close()
			return
		}
		_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: shippin-relay\r\nConnection: Upgrade\r\n\r\n")
		_ = brw.Flush()
		go func() { _, _ = io.Copy(up, brw); up.Close() }()
		_, _ = io.Copy(conn, up)
		conn.Close()
	}))
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	w.url = srv.URL
	w.caFile = filepath.Join(t.TempDir(), "console-ca.pem")
	_ = os.WriteFile(w.caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600)
	return w
}

// startSSHD answers `exec` with "ran: <command>" for clients holding a CA-signed cert for ws-w1.
func startSSHD(t *testing.T, ca ssh.PublicKey) string {
	_, hostKey, _ := ed25519.GenerateKey(rand.Reader)
	host, _ := ssh.NewSignerFromKey(hostKey)
	checker := &ssh.CertChecker{IsUserAuthority: func(k ssh.PublicKey) bool { return bytes.Equal(k.Marshal(), ca.Marshal()) }}
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		cert, ok := key.(*ssh.Certificate)
		if !ok {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, checker.CheckCert("ws-w1", cert)
	}}
	cfg.AddHostKey(host)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSSH(nc, cfg)
		}
	}()
	return ln.Addr().String()
}

func serveSSH(nc net.Conn, cfg *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		nc.Close()
		return
	}
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		ch, creqs, err := nch.Accept()
		if err != nil {
			continue
		}
		go func() {
			for req := range creqs {
				if req.Type != "exec" {
					_ = req.Reply(false, nil)
					continue
				}
				_ = req.Reply(true, nil)
				cmd, _ := bufio.NewReader(bytes.NewReader(req.Payload[4:])).ReadString(0)
				_, _ = io.WriteString(ch, "ran: "+cmd+"\n")
				_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
				ch.Close()
			}
		}()
	}
}
