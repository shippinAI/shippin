package proxmox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"shippin.cloud/console/internal/hv"
)

func (c *Client) Start(ctx context.Context, id string) (hv.Guest, error) {
	return c.act(ctx, id, "start", "running")
}

func (c *Client) Stop(ctx context.Context, id string) (hv.Guest, error) {
	return c.act(ctx, id, "stop", "stopped")
}

func (c *Client) Shutdown(ctx context.Context, id string) (hv.Guest, error) {
	return c.act(ctx, id, "shutdown", "stopped")
}

func (c *Client) Reboot(ctx context.Context, id string) (hv.Guest, error) {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return hv.Guest{}, err
	}
	if err := c.postStatusAndWait(ctx, g, "reboot"); err != nil {
		return hv.Guest{}, err
	}
	return c.Guest(ctx, id)
}

func (c *Client) act(ctx context.Context, id, verb, want string) (hv.Guest, error) {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return hv.Guest{}, err
	}
	if g.Status == want {
		return g, nil
	}
	if err := c.postStatusAndWait(ctx, g, verb); err != nil {
		return hv.Guest{}, err
	}
	got, err := c.Guest(ctx, id)
	if err != nil {
		return hv.Guest{}, err
	}
	if got.Status != want {
		return got, fmt.Errorf("proxmox: %s %s: hypervisor reports %q", verb, id, got.Status)
	}
	return got, nil
}

// postStatusAndWait posts verb and waits for the task it started. Reading the guest before this
// returns the old state: a start takes seconds on a real host.
func (c *Client) postStatusAndWait(ctx context.Context, g hv.Guest, verb string) error {
	body, err := c.postStatus(ctx, g, verb)
	if err != nil {
		return err
	}
	upid, err := upidOf(body)
	if err != nil {
		return err
	}
	_, err = c.Wait(ctx, upid)
	return err
}

func (c *Client) postStatus(ctx context.Context, g hv.Guest, verb string) ([]byte, error) {
	kind := "qemu"
	if g.Kind == hv.KindCT {
		kind = "lxc"
	}
	path := fmt.Sprintf("/nodes/%s/%s/%s/status/%s", c.nodeOf(g), kind, g.ID, verb)
	body, code, err := c.do(ctx, http.MethodPost, path, url.Values{})
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: %s HTTP %d: %s", verb, code, clip(body))
	}
	return body, nil
}

// Snapshot creates a qemu or LXC snapshot, then returns the guest from a fresh inventory read.
func (c *Client) Snapshot(ctx context.Context, id, name string) (hv.Guest, error) {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return hv.Guest{}, err
	}
	kind, err := apiKind(g)
	if err != nil {
		return hv.Guest{}, err
	}
	if !validSnap(name) {
		return hv.Guest{}, fmt.Errorf("proxmox: snapshot name %q is not a PVE snapname", name)
	}
	form := url.Values{}
	form.Set("snapname", name)
	form.Set("description", "snapshot from shippin-console")
	path := fmt.Sprintf("/nodes/%s/%s/%s/snapshot", c.nodeOf(g), kind, id)
	body, code, err := c.do(ctx, http.MethodPost, path, form)
	if err != nil {
		return hv.Guest{}, err
	}
	if code != http.StatusOK {
		return hv.Guest{}, fmt.Errorf("proxmox: snapshot HTTP %d: %s", code, clip(body))
	}
	upid, err := upidOf(body)
	if err != nil {
		return hv.Guest{}, err
	}
	if _, err := c.Wait(ctx, upid); err != nil {
		return hv.Guest{}, err
	}
	return c.Guest(ctx, id)
}

// Create posts a new qemu or lxc guest, waits for the UPID, and returns the inventory row.
func (c *Client) Create(ctx context.Context, spec Spec) (hv.Guest, error) {
	kind, err := kindPath(spec.Kind)
	if err != nil {
		return hv.Guest{}, err
	}
	form := url.Values{}
	form.Set("vmid", spec.ID)
	if spec.Name != "" {
		form.Set("hostname", spec.Name)
		form.Set("name", spec.Name)
	}
	if spec.Memory != "" {
		form.Set("memory", spec.Memory)
	}
	if spec.Cores != "" {
		form.Set("cores", spec.Cores)
	}
	if spec.Disk != "" {
		if kind == "qemu" {
			form.Set("scsi0", spec.Disk)
		} else {
			form.Set("rootfs", spec.Disk)
		}
	}
	if spec.Bridge != "" {
		form.Set("net0", "bridge="+spec.Bridge)
	}
	if spec.Archive != "" {
		form.Set("archive", spec.Archive)
	}
	upid, err := c.postUPID(ctx, http.MethodPost, "/nodes/"+c.Endpoint.Node+"/"+kind, form)
	if err != nil {
		return hv.Guest{}, err
	}
	if _, err := c.Wait(ctx, upid); err != nil {
		return hv.Guest{}, err
	}
	return c.Guest(ctx, spec.ID)
}

// Clone copies a guest, waits, and reads the new id back.
func (c *Client) Clone(ctx context.Context, id string, spec Spec) (hv.Guest, error) {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return hv.Guest{}, err
	}
	kind, err := apiKind(g)
	if err != nil {
		return hv.Guest{}, err
	}
	form := url.Values{}
	form.Set("newid", spec.NewID)
	if spec.Name != "" {
		form.Set("name", spec.Name)
	}
	if spec.Full() {
		form.Set("full", "1")
	}
	upid, err := c.postUPID(ctx, http.MethodPost, fmtPath(c.Endpoint.Node, kind, id, "clone"), form)
	if err != nil {
		return hv.Guest{}, err
	}
	if _, err := c.Wait(ctx, upid); err != nil {
		return hv.Guest{}, err
	}
	return c.Guest(ctx, spec.NewID)
}

// Full reports whether the clone should be a full copy. Empty means full.
func (s Spec) Full() bool { return s.Mode == "" || s.Mode == "full" }

// Template converts the guest, then reads it back.
func (c *Client) Template(ctx context.Context, id string) (hv.Guest, error) {
	return c.actionThenGuest(ctx, id, "template", id)
}

// Delete removes the guest. A following GET must be 404-equivalent (absent from inventory).
func (c *Client) Delete(ctx context.Context, id string) error {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return err
	}
	kind, err := apiKind(g)
	if err != nil {
		return err
	}
	upid, err := c.postUPID(ctx, http.MethodDelete, fmtPath(c.Endpoint.Node, kind, id, ""), nil)
	if err != nil {
		return err
	}
	if _, err := c.Wait(ctx, upid); err != nil {
		return err
	}
	_, err = c.Guest(ctx, id)
	if err == nil {
		return fmt.Errorf("proxmox: guest %s still present after delete", id)
	} else if !errors.Is(err, hv.ErrGuestNotFound) {
		return fmt.Errorf("proxmox: couldn't confirm guest %s was deleted: %w", id, err)
	}
	return nil
}

// Resize grows a disk, waits, and returns the guest.
func (c *Client) Resize(ctx context.Context, id, disk, size string) (hv.Guest, error) {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return hv.Guest{}, err
	}
	kind, err := apiKind(g)
	if err != nil {
		return hv.Guest{}, err
	}
	form := url.Values{}
	form.Set("disk", disk)
	form.Set("size", size)
	upid, err := c.postUPID(ctx, http.MethodPut, fmtPath(c.Endpoint.Node, kind, id, "resize"), form)
	if err != nil {
		return hv.Guest{}, err
	}
	if _, err := c.Wait(ctx, upid); err != nil {
		return hv.Guest{}, err
	}
	return c.Guest(ctx, id)
}

// Migrate moves a guest, waits, and reads it back.
func (c *Client) Migrate(ctx context.Context, id string, spec Spec) (hv.Guest, error) {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return hv.Guest{}, err
	}
	kind, err := apiKind(g)
	if err != nil {
		return hv.Guest{}, err
	}
	form := url.Values{}
	form.Set("target", spec.Target)
	if spec.Online {
		form.Set("online", "1")
	}
	upid, err := c.postUPID(ctx, http.MethodPost, fmtPath(c.Endpoint.Node, kind, id, "migrate"), form)
	if err != nil {
		return hv.Guest{}, err
	}
	if _, err := c.Wait(ctx, upid); err != nil {
		return hv.Guest{}, err
	}
	return c.Guest(ctx, id)
}

// Snapshots lists snapshot names.
func (c *Client) Snapshots(ctx context.Context, id string) ([]string, error) {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return nil, err
	}
	kind, err := apiKind(g)
	if err != nil {
		return nil, err
	}
	body, code, err := c.do(ctx, http.MethodGet, fmtPath(c.Endpoint.Node, kind, id, "snapshot"), nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: snapshots HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := []string{}
	for _, s := range r.Data {
		if s.Name != "" && s.Name != "current" {
			out = append(out, s.Name)
		}
	}
	return out, nil
}

// Rollback restores a snapshot and reads the guest back.
func (c *Client) Rollback(ctx context.Context, id, name string) (hv.Guest, error) {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return hv.Guest{}, err
	}
	kind, err := apiKind(g)
	if err != nil {
		return hv.Guest{}, err
	}
	upid, err := c.postUPID(ctx, http.MethodPost, fmtPath(c.Endpoint.Node, kind, id, "snapshot/"+url.PathEscape(name)+"/rollback"), url.Values{})
	if err != nil {
		return hv.Guest{}, err
	}
	if _, err := c.Wait(ctx, upid); err != nil {
		return hv.Guest{}, err
	}
	return c.Guest(ctx, id)
}

// Restore creates a guest from a backup archive and reads it back.
func (c *Client) Restore(ctx context.Context, spec Spec) (hv.Guest, error) {
	return c.Create(ctx, spec)
}

func (c *Client) actionThenGuest(ctx context.Context, id, action, readID string) (hv.Guest, error) {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return hv.Guest{}, err
	}
	kind, err := apiKind(g)
	if err != nil {
		return hv.Guest{}, err
	}
	upid, err := c.postUPID(ctx, http.MethodPost, fmtPath(c.Endpoint.Node, kind, id, action), url.Values{})
	if err != nil {
		return hv.Guest{}, err
	}
	if _, err := c.Wait(ctx, upid); err != nil {
		return hv.Guest{}, err
	}
	return c.Guest(ctx, readID)
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

func isBridge(n hv.Network) bool {
	return strings.EqualFold(n.Type, "bridge") || strings.EqualFold(n.Type, "ovsbridge")
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
