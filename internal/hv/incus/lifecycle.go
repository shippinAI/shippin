package incus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"shippin.cloud/console/internal/hv"
)

func (c *Client) Start(ctx context.Context, id string) (hv.Guest, error) {
	return c.act(ctx, id, "start", "Running")
}

func (c *Client) Stop(ctx context.Context, id string) (hv.Guest, error) {
	return c.act(ctx, id, "stop", "Stopped")
}

func (c *Client) Shutdown(ctx context.Context, id string) (hv.Guest, error) {
	return c.act(ctx, id, "stop", "Stopped")
}

func (c *Client) Reboot(ctx context.Context, id string) (hv.Guest, error) {
	if _, err := c.Guest(ctx, id); err != nil {
		return hv.Guest{}, err
	}
	if err := c.putState(ctx, id, "restart"); err != nil {
		return hv.Guest{}, err
	}
	return c.Guest(ctx, id)
}

func (c *Client) act(ctx context.Context, id, action, want string) (hv.Guest, error) {
	g, err := c.Guest(ctx, id)
	if err != nil {
		return hv.Guest{}, err
	}
	if strings.EqualFold(g.Status, want) {
		return g, nil
	}
	if err := c.putState(ctx, id, action); err != nil {
		return hv.Guest{}, err
	}
	got, err := c.Guest(ctx, id)
	if err != nil {
		return hv.Guest{}, err
	}
	if !strings.EqualFold(got.Status, want) {
		return got, fmt.Errorf("incus: %s %s: hypervisor reports %q", action, id, got.Status)
	}
	return got, nil
}

func (c *Client) putState(ctx context.Context, id, action string) error {
	raw, _ := json.Marshal(map[string]any{"action": action, "timeout": 30})
	body, code, err := c.do(ctx, http.MethodPut, c.path(instancePath(id)+"/state"), raw)
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusAccepted {
		return fmt.Errorf("incus: %s HTTP %d: %s", action, code, clip(body))
	}
	return c.wait(ctx, body)
}

func (c *Client) Snapshot(ctx context.Context, id, name string) (hv.Guest, error) {
	if _, err := c.Guest(ctx, id); err != nil {
		return hv.Guest{}, err
	}
	if name == "" || strings.Contains(name, "/") {
		return hv.Guest{}, fmt.Errorf("incus: snapshot name %q is invalid", name)
	}
	raw, _ := json.Marshal(map[string]string{"name": name})
	body, code, err := c.do(ctx, http.MethodPost, c.path(instancePath(id)+"/snapshots"), raw)
	if err != nil {
		return hv.Guest{}, err
	}
	if code != http.StatusOK && code != http.StatusAccepted && code != http.StatusCreated {
		return hv.Guest{}, fmt.Errorf("incus: snapshot HTTP %d: %s", code, clip(body))
	}
	if err := c.wait(ctx, body); err != nil {
		return hv.Guest{}, err
	}
	return c.Guest(ctx, id)
}

// Snapshots lists an instance's snapshots by name.
func (c *Client) Snapshots(ctx context.Context, id string) ([]string, error) {
	body, code, err := c.do(ctx, http.MethodGet, c.path(instancePath(id)+"/snapshots"), nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("incus: snapshots HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Metadata []string `json:"metadata"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("incus: snapshots: %w", err)
	}
	out := make([]string, 0, len(r.Metadata))
	for _, link := range r.Metadata {
		name := link[strings.LastIndex(link, "/")+1:]
		if plain, err := url.PathUnescape(name); err == nil {
			name = plain
		}
		out = append(out, name)
	}
	return out, nil
}

// Rollback restores a snapshot and reads the instance back.
func (c *Client) Rollback(ctx context.Context, id, name string) (hv.Guest, error) {
	if name == "" || strings.Contains(name, "/") {
		return hv.Guest{}, fmt.Errorf("incus: snapshot name %q is invalid", name)
	}
	raw, _ := json.Marshal(map[string]string{"restore": name})
	body, code, err := c.do(ctx, http.MethodPut, c.path(instancePath(id)), raw)
	if err != nil {
		return hv.Guest{}, err
	}
	if code != http.StatusOK && code != http.StatusAccepted {
		return hv.Guest{}, fmt.Errorf("incus: restore HTTP %d: %s", code, clip(body))
	}
	if err := c.wait(ctx, body); err != nil {
		return hv.Guest{}, err
	}
	return c.Guest(ctx, id)
}

// CreateGuest makes an instance and waits for it, which includes downloading its image the first time.
func (c *Client) CreateGuest(ctx context.Context, s hv.GuestSpec) (hv.Guest, error) {
	if !guestName.MatchString(s.Name) {
		return hv.Guest{}, errors.New("incus: a guest name starts with a letter and uses letters, digits and dashes")
	}
	source, err := imageSource(s.Image)
	if err != nil {
		return hv.Guest{}, err
	}
	kind := "container"
	if s.Kind == hv.KindVM {
		kind = "virtual-machine"
	}
	config := map[string]string{}
	if s.Memory > 0 {
		config["limits.memory"] = strconv.Itoa(s.Memory) + "MiB"
	}
	if s.Cores > 0 {
		config["limits.cpu"] = strconv.Itoa(s.Cores)
	}
	raw, err := json.Marshal(map[string]any{"name": s.Name, "type": kind, "source": source, "config": config})
	if err != nil {
		return hv.Guest{}, err
	}
	body, code, err := c.do(ctx, http.MethodPost, c.path("/1.0/instances"), raw)
	if err != nil {
		return hv.Guest{}, err
	}
	if code != http.StatusOK && code != http.StatusAccepted && code != http.StatusCreated {
		return hv.Guest{}, fmt.Errorf("incus: create HTTP %d: %s", code, clip(body))
	}
	if err := c.wait(ctx, body); err != nil {
		return hv.Guest{}, err
	}
	if s.Start {
		g, err := c.Start(ctx, s.Name)
		if err != nil {
			return g, fmt.Errorf("%s was created but did not start: %w", s.Name, err)
		}
		return g, nil
	}
	return c.Guest(ctx, s.Name)
}

// DeleteGuest stops an instance if it has to, removes it, and checks that it has gone.
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
	body, code, err := c.do(ctx, http.MethodDelete, c.path(instancePath(id)), nil)
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusAccepted {
		return fmt.Errorf("incus: delete HTTP %d: %s", code, clip(body))
	}
	if err := c.wait(ctx, body); err != nil {
		return err
	}
	if _, err := c.Guest(ctx, id); err == nil {
		return fmt.Errorf("incus: guest %s still present after delete", id)
	} else if !errors.Is(err, hv.ErrGuestNotFound) {
		return fmt.Errorf("incus: couldn't confirm guest %s was deleted: %w", id, err)
	}
	return nil
}
