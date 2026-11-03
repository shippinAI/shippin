package hostclient

import "time"

type Instance struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Type      string            `json:"type"`
	CreatedAt time.Time         `json:"created_at"`
	Config    map[string]string `json:"config"`
	State     *struct {
		Memory struct {
			Usage int64 `json:"usage"`
		} `json:"memory"`
		Processes int `json:"processes"`
		Network   map[string]struct {
			Addresses []struct {
				Family  string `json:"family"`
				Address string `json:"address"`
				Scope   string `json:"scope"`
			} `json:"addresses"`
		} `json:"network"`
	} `json:"state"`
}

// IPv4 returns the eth0 global IPv4 address, if any.
func (i Instance) IPv4() string {
	if i.State == nil {
		return ""
	}
	for _, a := range i.State.Network["eth0"].Addresses {
		if a.Family == "inet" && a.Scope == "global" {
			return a.Address
		}
	}
	return ""
}
