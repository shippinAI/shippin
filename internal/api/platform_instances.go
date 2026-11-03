package api

import (
	"context"
	"time"

	"shippin.cloud/console/internal/hostclient"
)

type instanceView struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Type        string    `json:"type"`
	IPv4        string    `json:"ipv4"`
	MemoryBytes int64     `json:"memory_bytes"`
	Processes   int       `json:"processes"`
	CreatedAt   time.Time `json:"created_at"`
	Image       string    `json:"image"`
	Autostart   bool      `json:"autostart"`
}

func toView(i hostclient.Instance) instanceView {
	v := instanceView{Name: i.Name, Status: i.Status, Type: i.Type, IPv4: i.IPv4(), CreatedAt: i.CreatedAt,
		Image:     i.Config["image.os"] + " " + i.Config["image.release"],
		Autostart: i.Config["boot.autostart"] == "true"}
	if i.State != nil {
		v.MemoryBytes, v.Processes = i.State.Memory.Usage, i.State.Processes
	}
	return v
}

func (a *API) instances(ctx context.Context) ([]instanceView, error) {
	raw, err := a.host.PlatformInstances(ctx)
	out := make([]instanceView, 0, len(raw))
	for _, i := range raw {
		out = append(out, toView(i))
	}
	return out, err
}
