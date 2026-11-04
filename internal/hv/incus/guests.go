package incus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"shippin.cloud/console/internal/hv"
)

var guestName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,62}$`)

// publicImages is the image server Incus itself documents, so "images:debian/13" means what it means
// on the command line. Images named this way come from configuration.
const publicImages = "https://images.linuxcontainers.org"

// imageSource turns an image id into what Incus wants: "images:<alias>" from the public server, or
// "local:<fingerprint>" from this server.
func imageSource(id string) (map[string]string, error) {
	if alias, ok := strings.CutPrefix(id, "images:"); ok && alias != "" {
		return map[string]string{"type": "image", "mode": "pull", "protocol": "simplestreams", "server": publicImages, "alias": alias}, nil
	}
	if fp, ok := strings.CutPrefix(id, "local:"); ok && fp != "" {
		return map[string]string{"type": "image", "fingerprint": fp}, nil
	}
	return nil, errors.New("incus: choose an image for this guest")
}

// Images lists what this server already has. Those it can fetch come from configuration.
func (c *Client) Images(ctx context.Context) ([]hv.Image, error) {
	body, code, err := c.do(ctx, http.MethodGet, c.path("/1.0/images", "recursion", "1"), nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("incus: images HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Metadata []struct {
			Fingerprint string `json:"fingerprint"`
			Type        string `json:"type"`
			Properties  struct {
				Description string `json:"description"`
			} `json:"properties"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("incus: images: %w", err)
	}
	out := make([]hv.Image, 0, len(r.Metadata))
	for _, m := range r.Metadata {
		label := m.Properties.Description
		if label == "" {
			label = m.Fingerprint[:min(12, len(m.Fingerprint))]
		}
		kind := hv.KindCT
		if m.Type == "virtual-machine" {
			kind = hv.KindVM
		}
		out = append(out, hv.Image{ID: "local:" + m.Fingerprint, Label: label, Kind: kind})
	}
	return out, nil
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
