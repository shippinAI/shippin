// Package enroll is the hostd side of CP-P1: redeem a single-use enrollment token once, keep the
// host credential in a 0600 file, and send a heartbeat every interval the console dictates.
// Nothing here prints a token or a credential.
package enroll

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Config comes from hostd.json; every value is required when enrollment is on.
type Config struct {
	ConsoleURL     string `json:"console_url"`
	TokenFile      string `json:"token_file"`
	CredentialFile string `json:"credential_file"`
}

// ErrRejected: the console refused the token or credential; a new token is needed.
var ErrRejected = errors.New("console rejected the enrollment token or host credential")

// saved is the credential file (0600). It never leaves the host.
type saved struct {
	ServerID         string `json:"server_id"`
	Credential       string `json:"credential"`
	HeartbeatSeconds int    `json:"heartbeat_seconds"`
}

// Agent enrolls and beats. HTTP and Hostname are seams for tests.
type Agent struct {
	Cfg      Config
	Version  string
	Hostname func() (string, error)
	HTTP     *http.Client
}

func (a *Agent) client() *http.Client {
	client := &http.Client{Timeout: 10 * time.Second}
	if a.HTTP != nil {
		copy := *a.HTTP
		client = &copy
	}
	// Enrollment and heartbeat carry bearer credentials. Treat redirects as
	// endpoint failures so credentials and host identity never follow a
	// server-controlled Location header, including same-origin redirects.
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client
}

// post sends body to the console; a 401 is ErrRejected, other non-2xx are retryable errors.
func (a *Agent) post(ctx context.Context, path, bearer string, body, out any) error {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Cfg.ConsoleURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := a.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return ErrRejected
	case res.StatusCode/100 != 2:
		return fmt.Errorf("console %s: HTTP %d", path, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// Enroll redeems the token file once, writes the credential file (0600) and removes the token.
func (a *Agent) Enroll(ctx context.Context) (*saved, error) {
	if s, err := a.load(); err == nil {
		return s, nil
	}
	tok, err := os.ReadFile(a.Cfg.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("no credential and no enrollment token file: %w", err)
	}
	host, _ := a.hostname()
	var s saved
	in := map[string]string{"token": string(bytes.TrimSpace(tok)), "hostname": host, "version": a.Version}
	if err := a.post(ctx, "/v1/servers/enroll", "", in, &s); err != nil {
		return nil, err
	}
	if err := writeSecret(a.Cfg.CredentialFile, s); err != nil {
		return nil, err
	}
	_ = os.Remove(a.Cfg.TokenFile)
	return &s, nil
}

func (a *Agent) hostname() (string, error) {
	if a.Hostname != nil {
		return a.Hostname()
	}
	return os.Hostname()
}

func (a *Agent) load() (*saved, error) {
	raw, err := os.ReadFile(a.Cfg.CredentialFile)
	if err != nil {
		return nil, err
	}
	var s saved
	if err := json.Unmarshal(raw, &s); err != nil || s.Credential == "" || s.HeartbeatSeconds < 1 {
		return nil, errors.New("credential file unreadable")
	}
	return &s, nil
}

// writeSecret writes v to path with mode 0600 via a temp file and rename.
func writeSecret(path string, v any) error {
	raw, _ := json.Marshal(v)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
