// Package gh talks to GitHub as the Shippin GitHub App: App JWTs, narrowed installation tokens,
// the install-time OAuth check, webhook verification and repository listing. Stdlib only.
package gh

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// App is the configured GitHub App. Secrets stay in this struct and never reach a log line.
type App struct {
	ClientID     string
	ClientSecret string
	API, Web     string
	HTTP         *http.Client
	key          *rsa.PrivateKey
	now          func() time.Time
	mu           sync.Mutex
	cache        map[string]cached
}

// New parses the App's PEM private key (PKCS#1 or PKCS#8).
func New(clientID, clientSecret, pemKey, api, web string) (*App, error) {
	b, _ := pem.Decode([]byte(pemKey))
	if b == nil {
		return nil, errors.New("github private key is not PEM")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(b.Bytes); err == nil {
		key = k
	} else if k, err := x509.ParsePKCS8PrivateKey(b.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("github private key is not RSA")
		}
		key = rk
	} else {
		return nil, errors.New("github private key does not parse")
	}
	return &App{ClientID: clientID, ClientSecret: clientSecret, API: strings.TrimRight(api, "/"), Web: strings.TrimRight(web, "/"),
		HTTP: &http.Client{Timeout: 20 * time.Second}, key: key, now: time.Now, cache: map[string]cached{}}, nil
}

// StatusError is a non-2xx GitHub answer (body withheld: it may echo request data).
type StatusError struct{ Code int }

func (e *StatusError) Error() string { return fmt.Sprintf("github answered %d", e.Code) }

// call sends one request with the given Authorization and decodes JSON into out (≤4 MiB).
func (a *App) call(ctx context.Context, method, url, auth string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, _ := json.Marshal(in)
		body = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := a.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return &StatusError{Code: res.StatusCode}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out)
}
