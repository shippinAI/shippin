package goldexport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Client talks to the console. The bearer only reads envelopes and replaces synced facts.
type Client struct {
	Base  string // https only, e.g. https://console.<domain> (CheckURL refuses http)
	Token string
	HTTP  *http.Client
}

// CheckURL allows https only, tailnet addresses included: the bearer never rides plain http.
func CheckURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("console URL must be https://")
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Base, "/")+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s %s: HTTP %d", method, path, res.StatusCode) // body not echoed
	}
	return b, nil
}

// Push sends one snapshot.
func (c *Client) Push(ctx context.Context, s *Sync) error {
	_, err := c.do(ctx, http.MethodPut, "/v1/gold-sign/sync", s)
	return err
}

// Pull writes each queued envelope into the root-only inbox for gold-passkey-verify drain.
func (c *Client) Pull(ctx context.Context, inbox string) (int, error) {
	b, err := c.do(ctx, http.MethodGet, "/v1/gold-sign/outbox", nil)
	if err != nil {
		return 0, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(b, &items); err != nil {
		return 0, err
	}
	for i, it := range items {
		if len(it) > 64<<10 {
			continue
		}
		f := filepath.Join(inbox, fmt.Sprintf("%d-%d.json", time.Now().UnixNano(), i))
		if err := os.WriteFile(f, it, 0o600); err != nil {
			return i, err
		}
	}
	return len(items), nil
}
