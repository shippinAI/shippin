package proxmox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"shippin.cloud/console/internal/hv"
)

var guestName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,62}$`)

func hasContent(v hv.Volume, kind string) bool {
	for _, c := range strings.Split(v.Content, ",") {
		if c == kind {
			return true
		}
	}
	return false
}

func isBridge(n hv.Network) bool {
	return strings.EqualFold(n.Type, "bridge") || strings.EqualFold(n.Type, "ovsbridge")
}

// Images lists the container templates and installer images already on this node's storage.
func (c *Client) Images(ctx context.Context) ([]hv.Image, error) {
	vols, err := c.Storage(ctx)
	if err != nil {
		return nil, err
	}
	out := []hv.Image{}
	for _, v := range vols {
		for _, want := range []struct{ content, kind string }{{"vztmpl", hv.KindCT}, {"iso", hv.KindVM}} {
			if !hasContent(v, want.content) {
				continue
			}
			files, err := c.Content(ctx, v.Name, want.content)
			if err != nil {
				return nil, err
			}
			for _, f := range files {
				out = append(out, hv.Image{ID: f, Label: f[strings.LastIndex(f, "/")+1:], Kind: want.kind})
			}
		}
	}
	return out, nil
}

func (c *Client) nextID(ctx context.Context) (string, error) {
	body, code, err := c.do(ctx, http.MethodGet, "/cluster/nextid", nil)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("proxmox: next id HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	return strings.Trim(string(r.Data), "\""), nil
}

func (c *Client) storageFor(ctx context.Context, name, content string) (string, error) {
	vols, err := c.Storage(ctx)
	if err != nil {
		return "", err
	}
	for _, v := range vols {
		if v.Name == name && hasContent(v, content) {
			return v.Name, nil
		}
	}
	return "", fmt.Errorf("proxmox: storage %q on this node does not provide %s content", name, content)
}

func (c *Client) bridge(ctx context.Context, name string) (string, error) {
	nets, err := c.Networks(ctx)
	if err != nil {
		return "", err
	}
	for _, n := range nets {
		if n.Name == name && isBridge(n) {
			return n.Name, nil
		}
	}
	return "", fmt.Errorf("proxmox: network %q is not a bridge on the configured node", name)
}

// CreateGuest makes a container from a template, or a virtual machine with an empty disk and an
// optional installer image. Placement is explicit; it never guesses storage or a bridge.
func (c *Client) CreateGuest(ctx context.Context, s hv.GuestSpec) (hv.Guest, error) {
	if !guestName.MatchString(s.Name) {
		return hv.Guest{}, errors.New("proxmox: a guest name starts with a letter and uses letters, digits and dashes")
	}
	if s.Kind == hv.KindCT && s.Image == "" {
		return hv.Guest{}, errors.New("proxmox: a container needs a template; download one to this node first")
	}
	if s.Disk <= 0 {
		return hv.Guest{}, errors.New("proxmox: disk size must be greater than zero GiB")
	}
	id, err := c.nextID(ctx)
	if err != nil {
		return hv.Guest{}, err
	}
	bridge, err := c.bridge(ctx, s.Network)
	if err != nil {
		return hv.Guest{}, err
	}
	disk := s.Disk
	form := url.Values{}
	form.Set("vmid", id)
	if s.Memory > 0 {
		form.Set("memory", strconv.Itoa(s.Memory))
	}
	if s.Cores > 0 {
		form.Set("cores", strconv.Itoa(s.Cores))
	}
	kind := "qemu"
	if s.Kind == hv.KindCT {
		kind = "lxc"
		storage, err := c.storageFor(ctx, s.Storage, "rootdir")
		if err != nil {
			return hv.Guest{}, err
		}
		form.Set("hostname", s.Name)
		form.Set("ostemplate", s.Image)
		form.Set("rootfs", storage+":"+strconv.Itoa(disk))
		form.Set("net0", "name=eth0,bridge="+bridge+",ip=dhcp")
	} else {
		storage, err := c.storageFor(ctx, s.Storage, "images")
		if err != nil {
			return hv.Guest{}, err
		}
		form.Set("name", s.Name)
		form.Set("scsi0", storage+":"+strconv.Itoa(disk))
		form.Set("net0", "virtio,bridge="+bridge)
		if s.Image != "" {
			form.Set("ide2", s.Image+",media=cdrom")
			form.Set("boot", "order=scsi0;ide2")
		}
	}
	upid, err := c.postUPID(ctx, http.MethodPost, "/nodes/"+c.Endpoint.Node+"/"+kind, form)
	if err != nil {
		return hv.Guest{}, err
	}
	if _, err := c.Wait(ctx, upid); err != nil {
		return hv.Guest{}, err
	}
	if err := c.awaitInventory(ctx, id); err != nil {
		return hv.Guest{}, err
	}
	if s.Start {
		g, err := c.Start(ctx, id)
		if err != nil {
			return g, fmt.Errorf("%s was created as %s but did not start: %w", s.Name, id, err)
		}
		return g, nil
	}
	return c.Guest(ctx, id)
}

// refusal is the reason Proxmox gives: its message and, for a bad parameter, which one.
func refusal(body []byte) string {
	var r struct {
		Message string            `json:"message"`
		Errors  map[string]string `json:"errors"`
	}
	if json.Unmarshal(body, &r) != nil || r.Message == "" {
		return clip(body)
	}
	out := []string{strings.TrimSpace(r.Message)}
	for _, k := range slices.Sorted(maps.Keys(r.Errors)) {
		out = append(out, k+": "+strings.TrimSpace(r.Errors[k]))
	}
	return strings.Join(out, " ")
}

// awaitInventory waits for a new guest to appear. /cluster/resources catches up about every ten seconds.
func (c *Client) awaitInventory(ctx context.Context, id string) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, err := c.Guest(ctx, id)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// DeleteGuest stops a guest if it has to, removes it, and checks that it has gone.
func (c *Client) DeleteGuest(ctx context.Context, id string) error {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return err
	}
	if !strings.EqualFold(g.Status, "stopped") {
		if _, err := c.Stop(ctx, id); err != nil {
			return err
		}
	}
	return c.Delete(ctx, id)
}
