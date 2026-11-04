package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"shippin.cloud/console/internal/hv"
)

// Task is one Proxmox UPID. Done is true when status is stopped.
type Task struct {
	UPID   string   `json:"upid"`
	Status string   `json:"status"`
	Exit   string   `json:"exitstatus"`
	Type   string   `json:"type"`
	Log    []string `json:"log,omitempty"`
	Done   bool     `json:"done"`
}

// Spec is the create/clone input. Fields the API does not need stay empty.
type Spec struct {
	Kind    string // vm or ct
	ID      string
	Name    string
	Memory  string
	Cores   string
	Disk    string
	Bridge  string
	NewID   string
	Target  string
	Online  bool
	Storage string
	Mode    string
	Archive string
	URL     string
	Content string
	File    string
}

// Task fetches one UPID status and the log lines available now.
func (c *Client) Task(ctx context.Context, upid string) (Task, error) {
	path := fmt.Sprintf("/nodes/%s/tasks/%s/status", c.Endpoint.Node, url.PathEscape(upid))
	body, code, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return Task{}, err
	}
	if code != http.StatusOK {
		return Task{}, fmt.Errorf("proxmox: task HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Data struct {
			UPID   string `json:"upid"`
			Status string `json:"status"`
			Exit   string `json:"exitstatus"`
			Type   string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return Task{}, err
	}
	t := Task{UPID: r.Data.UPID, Status: r.Data.Status, Exit: r.Data.Exit, Type: r.Data.Type, Done: r.Data.Status == "stopped"}
	if t.UPID == "" {
		t.UPID = upid
	}
	logBody, logCode, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/nodes/%s/tasks/%s/log", c.Endpoint.Node, url.PathEscape(upid)), nil)
	if err == nil && logCode == http.StatusOK {
		var lg struct {
			Data []struct {
				Text string `json:"t"`
			} `json:"data"`
		}
		if json.Unmarshal(logBody, &lg) == nil {
			for _, line := range lg.Data {
				t.Log = append(t.Log, line.Text)
			}
		}
	}
	return t, nil
}

// Wait polls the UPID with exponential backoff until it stops or the context ends.
func (c *Client) Wait(ctx context.Context, upid string) (Task, error) {
	wait := c.PollBase
	if wait <= 0 {
		wait = 200 * time.Millisecond
	}
	max := c.PollMax
	if max <= 0 {
		max = 5 * time.Second
	}
	var last string
	for {
		t, err := c.Task(ctx, upid)
		if err != nil {
			return t, err
		}
		if t.Done {
			if t.Exit != "" && t.Exit != "OK" {
				return t, fmt.Errorf("proxmox: task %s exit %s", upid, t.Exit)
			}
			return t, nil
		}
		if t.Status != last {
			last = t.Status
		}
		select {
		case <-ctx.Done():
			return t, ctx.Err()
		case <-time.After(wait):
		}
		if wait < max {
			wait *= 2
			if wait > max {
				wait = max
			}
		}
	}
}

// Config is the raw PVE config object.
func (c *Client) Config(ctx context.Context, id string) (map[string]any, error) {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return nil, err
	}
	kind, err := apiKind(g)
	if err != nil {
		return nil, err
	}
	body, code, err := c.do(ctx, http.MethodGet, fmtPath(c.Endpoint.Node, kind, id, "config"), nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: config HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return r.Data, nil
}

// SetConfig writes config keys and reads them back.
func (c *Client) SetConfig(ctx context.Context, id string, values map[string]string) (map[string]any, error) {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return nil, err
	}
	kind, err := apiKind(g)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	for k, v := range values {
		form.Set(k, v)
	}
	body, code, err := c.do(ctx, http.MethodPut, fmtPath(c.Endpoint.Node, kind, id, "config"), form)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: config HTTP %d: %s", code, clip(body))
	}
	return c.Config(ctx, id)
}

// Content lists files on a storage.
func (c *Client) Content(ctx context.Context, storage, content string) ([]string, error) {
	path := fmt.Sprintf("/nodes/%s/storage/%s/content", c.Endpoint.Node, url.PathEscape(storage))
	if content != "" {
		path += "?content=" + url.QueryEscape(content)
	}
	body, code, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: content HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Data []struct {
			Volid string `json:"volid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(r.Data))
	for _, v := range r.Data {
		out = append(out, v.Volid)
	}
	return out, nil
}

// DownloadURL asks the node to fetch an ISO or template. The URL is an argument, not a constant.
func (c *Client) DownloadURL(ctx context.Context, spec Spec) (string, error) {
	if !strings.HasPrefix(spec.URL, "https://") {
		return "", fmt.Errorf("proxmox: download URL must be https")
	}
	form := url.Values{}
	form.Set("url", spec.URL)
	form.Set("content", spec.Content)
	form.Set("filename", spec.File)
	return c.postUPID(ctx, http.MethodPost, fmt.Sprintf("/nodes/%s/storage/%s/download-url", c.Endpoint.Node, url.PathEscape(spec.Storage)), form)
}

// Upload sends an ISO or template body. The caller supplies the bytes.
func (c *Client) Upload(ctx context.Context, storage, content, filename string, r io.Reader) (string, error) {
	var buf strings.Builder
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("content", content)
	part, err := w.CreateFormFile("filename", filename)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, r); err != nil {
		return "", err
	}
	_ = w.Close()
	secret, err := c.Secret()
	if err != nil {
		return "", err
	}
	u := strings.TrimRight(c.Endpoint.BaseURL, "/") + "/api2/json/nodes/" + c.Endpoint.Node + "/storage/" + url.PathEscape(storage) + "/upload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(buf.String()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "PVEAPIToken="+c.Endpoint.TokenID+"="+secret)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("proxmox: upload HTTP %d: %s", res.StatusCode, clip(b))
	}
	return upidOf(b)
}

// Services lists node services.
func (c *Client) Services(ctx context.Context) ([]string, error) {
	return c.stringList(ctx, "/nodes/"+c.Endpoint.Node+"/services", "service")
}

// NodeNetworks lists bridges, bonds, and VLANs.
func (c *Client) NodeNetworks(ctx context.Context) ([]hv.Network, error) {
	return c.Networks(ctx)
}

// FirewallRules reads node or guest rules. guest empty means the node.
func (c *Client) FirewallRules(ctx context.Context, guest string) ([]string, error) {
	path := "/nodes/" + c.Endpoint.Node + "/firewall/rules"
	if guest != "" {
		g, err := c.Guest(ctx, guest)
		if err != nil {
			return nil, err
		}
		kind, err := apiKind(g)
		if err != nil {
			return nil, err
		}
		path = fmtPath(c.Endpoint.Node, kind, guest, "firewall/rules")
	}
	body, code, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: firewall HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Data []struct {
			Type   string `json:"type"`
			Action string `json:"action"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(r.Data))
	for _, rule := range r.Data {
		out = append(out, rule.Type+" "+rule.Action)
	}
	return out, nil
}

// Vzdump starts a backup task and returns its UPID after the API accepts it.
func (c *Client) Vzdump(ctx context.Context, spec Spec) (string, error) {
	form := url.Values{}
	form.Set("vmid", spec.ID)
	form.Set("storage", spec.Storage)
	if spec.Mode != "" {
		form.Set("mode", spec.Mode)
	}
	return c.postUPID(ctx, http.MethodPost, "/nodes/"+c.Endpoint.Node+"/vzdump", form)
}

// Backups lists backup volumes on a storage (PBS or vzdump content).
func (c *Client) Backups(ctx context.Context, storage string) ([]string, error) {
	return c.Content(ctx, storage, "backup")
}

func (c *Client) postUPID(ctx context.Context, method, path string, form url.Values) (string, error) {
	body, code, err := c.do(ctx, method, path, form)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("proxmox: HTTP %d: %s", code, refusal(body))
	}
	return upidOf(body)
}

func (c *Client) stringList(ctx context.Context, path, field string) ([]string, error) {
	body, code, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(r.Data))
	for _, row := range r.Data {
		if s, ok := row[field].(string); ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func upidOf(body []byte) (string, error) {
	var r struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal(r.Data, &s); err != nil {
		return "", fmt.Errorf("proxmox: response was not a UPID")
	}
	if !strings.HasPrefix(s, "UPID:") {
		return "", fmt.Errorf("proxmox: %q is not a UPID", s)
	}
	return s, nil
}

func kindPath(kind string) (string, error) {
	switch kind {
	case hv.KindVM, "qemu":
		return "qemu", nil
	case hv.KindCT, "lxc":
		return "lxc", nil
	default:
		return "", fmt.Errorf("proxmox: kind must be vm or ct")
	}
}

func fmtPath(node, kind, id, rest string) string {
	p := "/nodes/" + node + "/" + kind + "/" + id
	if rest != "" {
		p += "/" + rest
	}
	return p
}
