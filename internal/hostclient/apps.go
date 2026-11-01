package hostclient

import (
	"context"
	"net/http"
	"time"

	"shippin.cloud/console/internal/apps"
)

// AppsStatus reads the tenant VM's install probe. Signed-in is always unknown.
func (c *Client) AppsStatus(ctx context.Context, t string) (*apps.Status, error) {
	out := &apps.Status{}
	if err := c.do(ctx, http.MethodGet, "/tenants/"+t+"/apps/status", nil, out); err != nil {
		return nil, err
	}
	apps.Normalize(out)
	return out, nil
}

// DesktopPrepare installs the VM desktop prerequisites (not the AI apps). Apt can take a while,
// so this uses its own client instead of the six-minute default.
func (c *Client) DesktopPrepare(ctx context.Context, t string) error {
	slow := &Client{Base: c.Base, HTTP: &http.Client{Timeout: 20 * time.Minute}}
	return slow.do(ctx, http.MethodPost, "/tenants/"+t+"/desktop/prepare", nil, nil)
}
