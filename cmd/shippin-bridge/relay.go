package main

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/crypto/ssh"
)

// dialRelay opens the authenticated relay to a workspace's sshd through the console (TLS).
func dialRelay(c creds, ws string) (net.Conn, error) {
	u, err := url.Parse(c.Server)
	if err != nil {
		return nil, err
	}
	host := u.Host
	if u.Port() == "" {
		host += ":443"
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", host, &tls.Config{ServerName: u.Hostname()})
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(conn, "GET /v1/bridge/workspaces/%s/relay HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nConnection: Upgrade\r\nUpgrade: shippin-relay\r\n\r\n", ws, u.Host, c.Token)
	br := bufio.NewReader(conn)
	res, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if res.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, &apiError{res.StatusCode, "relay refused"}
	}
	return &bufConn{conn, br}, nil
}

type bufConn struct {
	net.Conn
	r *bufio.Reader
}

func (b *bufConn) Read(p []byte) (int, error) { return b.r.Read(p) }

// resolve accepts a workspace id or name.
func resolve(c creds, ref string) (workspace, error) {
	ws, _, err := workspaces(c)
	if err != nil {
		return workspace{}, err
	}
	for _, w := range ws {
		if w.ID == ref || w.Name == ref {
			return w, nil
		}
	}
	return workspace{}, errors.New("no workspace " + ref + " for this account (shippin-bridge ls)")
}

// connect signs a fresh in-memory key for one workspace and opens SSH over the relay. The private
// key never touches disk; the certificate expires in minutes.
func connect(c creds, ref string) (*ssh.Client, workspace, error) {
	w, err := resolve(c, ref)
	if err != nil {
		return nil, w, err
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sp, _ := ssh.NewPublicKey(pub)
	var cert struct {
		Certificate string `json:"certificate"`
		User        string `json:"ssh_user"`
	}
	if err := call(c, "POST", "/v1/bridge/workspaces/"+w.ID+"/cert", map[string]string{"public_key": string(ssh.MarshalAuthorizedKey(sp))}, &cert); err != nil {
		return nil, w, err
	}
	k, _, _, _, err := ssh.ParseAuthorizedKey([]byte(cert.Certificate))
	if err != nil {
		return nil, w, err
	}
	base, _ := ssh.NewSignerFromKey(priv)
	signer, err := ssh.NewCertSigner(k.(*ssh.Certificate), base)
	if err != nil {
		return nil, w, err
	}
	conn, err := dialRelay(c, w.ID)
	if err != nil {
		return nil, w, err
	}
	cfg := &ssh.ClientConfig{User: cert.User, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, Timeout: 15 * time.Second,
		// The relay is TLS to the console, which reaches only this workspace's sshd via hostd.
		HostKeyCallback: ssh.InsecureIgnoreHostKey()}
	sc, chans, reqs, err := ssh.NewClientConn(conn, w.ID, cfg)
	if err != nil {
		conn.Close()
		return nil, w, err
	}
	return ssh.NewClient(sc, chans, reqs), w, nil
}
