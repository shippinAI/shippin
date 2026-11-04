package incus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
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
