package main

// OpenSSH access for tools that run the system ssh client, such as Claude Desktop's SSH
// environments and editors' remote modes. `ssh-config` prints one config block; inside it,
// `Match ... exec` runs `cert` before every connection, which keeps a short-lived certificate
// on disk, and ProxyCommand runs `proxy`, the same relay `shippin-bridge ssh` uses. No static
// key and no open port: the certificate expires in minutes and the relay needs the bridge sign-in.

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// certReuse: an existing certificate with at least this much life left is used as it is,
// so tools that open many connections don't ask the console for a new one each time.
const certReuse = 3 * time.Minute

// bridgeDir is ~/.shippin/bridge; certificates live in its ssh/ folder.
func bridgeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".shippin", "bridge"), nil
}

// certCmd makes sure ~/.shippin/bridge/ssh/<id> and <id>-cert.pub hold a live certificate
// for one workspace. It prints nothing on success, as Match exec expects.
func certCmd(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: shippin-bridge cert <workspace id>")
	}
	_, err := ensureCert(args[0], false)
	return err
}

// ensureCert returns the ssh user when it signs a new certificate ("" when it reuses one).
func ensureCert(ref string, fresh bool) (string, error) {
	top, err := bridgeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(top, "ssh")
	if !fresh && certLife(filepath.Join(dir, ref+"-cert.pub")) > certReuse {
		return "", nil
	}
	c, err := loadCreds()
	if err != nil {
		return "", err
	}
	w, err := resolve(c, ref)
	if err != nil {
		return "", err
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sp, _ := ssh.NewPublicKey(pub)
	var cert struct {
		Certificate string `json:"certificate"`
		User        string `json:"ssh_user"`
	}
	if err := call(c, "POST", "/v1/bridge/workspaces/"+w.ID+"/cert", map[string]string{"public_key": string(ssh.MarshalAuthorizedKey(sp))}, &cert); err != nil {
		return "", err
	}
	block, err := ssh.MarshalPrivateKey(priv, "shippin-bridge "+w.Name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := filepath.Join(dir, w.ID)
	if err := writePrivate(base, pem.EncodeToMemory(block)); err != nil {
		return "", err
	}
	return cert.User, writePrivate(base+"-cert.pub", []byte(strings.TrimSpace(cert.Certificate)+"\n"))
}

// certLife is the time left on a certificate file; zero when it is missing or unreadable.
func certLife(path string) time.Duration {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	k, _, _, _, err := ssh.ParseAuthorizedKey(raw)
	if err != nil {
		return 0
	}
	sc, ok := k.(*ssh.Certificate)
	if !ok {
		return 0
	}
	return time.Until(time.Unix(int64(sc.ValidBefore), 0))
}

// writePrivate replaces a file atomically, readable by its owner only.
func writePrivate(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// sshConfig prints the block for one workspace. Put it before any `Host *` block: ssh uses the
// first value it finds for each setting.
func sshConfig(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: shippin-bridge ssh-config <workspace>  (then append the output to ~/.ssh/config)")
	}
	c, err := loadCreds()
	if err != nil {
		return err
	}
	w, err := resolve(c, args[0])
	if err != nil {
		return err
	}
	bridge, err := bridgeCommand()
	if err != nil {
		return err
	}
	user, err := ensureCert(w.ID, true)
	if err != nil {
		return err
	}
	dir, err := bridgeDir()
	if err != nil {
		return err
	}
	fmt.Print(configBlock(w, user, bridge, dir))
	return nil
}
