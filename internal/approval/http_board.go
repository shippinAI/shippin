package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPBoard is a client for a remote agent hub that serves POST/GET /v1/hub/approvals.
// shippin-cloud and shippin-platform were not readable from this workspace, so this is the
// contract this console itself serves. See docs/approval.md.
type HTTPBoard struct {
	BaseURL string
	Token   string
	OrgID   string
	HTTP    *http.Client
}

func (h HTTPBoard) client() *http.Client {
	if h.HTTP != nil {
		return h.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// Post creates or refreshes an approval card.
func (h HTTPBoard) Post(ctx context.Context, req Request) (Decision, error) {
	if req.OrgID == "" {
		req.OrgID = h.OrgID
	}
	var d Decision
	err := h.do(ctx, http.MethodPost, "/v1/hub/approvals", req, &d)
	return d, err
}

// Status reads the card for key.
func (h HTTPBoard) Status(ctx context.Context, org, key string) (Decision, error) {
	if org == "" {
		org = h.OrgID
	}
	var d Decision
	err := h.do(ctx, http.MethodGet, "/v1/hub/approvals/"+url.PathEscape(key)+"?org="+url.QueryEscape(org), nil, &d)
	return d, err
}

func (h HTTPBoard) do(ctx context.Context, method, path string, body, out any) error {
	base := strings.TrimRight(h.BaseURL, "/")
	if base == "" {
		return fmt.Errorf("approval board: base_url is empty")
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	r, err := http.NewRequestWithContext(ctx, method, base+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if h.Token != "" {
		r.Header.Set("Authorization", "Bearer "+h.Token)
	}
	res, err := h.client().Do(r)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("approval board: %s %s: %d", method, path, res.StatusCode)
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}
