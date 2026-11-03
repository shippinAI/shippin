package hostclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ChatRun starts a named chat command in a workspace with prompt on stdin (session: the console chat id, for session commands) and returns hostd's
// streamed output (NDJSON/text, ending with a {"shippin_exit":…} trailer). The caller closes it.
func (c *Client) ChatRun(ctx context.Context, t, w, cmd, session, prompt string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/tenants/"+t+"/workspaces/"+w+"/chat/"+cmd+"?session="+url.QueryEscape(session), strings.NewReader(prompt))
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("hostd chat %s: %s", cmd, res.Status)
	}
	return res.Body, nil
}
