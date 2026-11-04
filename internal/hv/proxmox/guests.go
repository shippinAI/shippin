package proxmox

import (
	"context"
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strings"

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
