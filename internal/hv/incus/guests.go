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

// imageSource turns a chosen image into what Incus wants: an alias on the configured image server named
// in the spec, or "local:<fingerprint>" from this server.
func imageSource(s hv.GuestSpec) (map[string]string, error) {
	if s.ImageServer != "" && s.Image != "" {
		return map[string]string{"type": "image", "mode": "pull", "protocol": s.ImageProtocol, "server": s.ImageServer, "alias": s.Image}, nil
	}
	if fp, ok := strings.CutPrefix(s.Image, "local:"); ok && fp != "" {
		return map[string]string{"type": "image", "fingerprint": fp}, nil
	}
	return nil, errors.New("incus: choose an image for this guest")
}

// Images lists what this server already has. Those it can fetch are named in configuration.
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
