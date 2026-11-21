package acquisition_source

import (
	"shippin.cloud/console/internal/acquisition"
	"shippin.cloud/console/internal/mcpl"
)

type UpdateInput struct {
	ByID
	ExpectedConfigGeneration int64  `json:"expected_config_generation"`
	Label                    string `json:"label"`
	Origin                   string `json:"origin"`
	DialIP                   string `json:"dial_ip"`
	Port                     int    `json:"port"`
	CAPEM                    string `json:"ca_pem"`
	SigningKey               string `json:"signing_key,omitempty"`
	Paused                   *bool  `json:"paused"`
}

var Update = mcpl.Def[UpdateInput, Summary]{
	Spec: mcpl.Spec{Summary: "Update a source endpoint or trust settings with an expected generation and explicit pause state.", Access: mcpl.Staff, Perm: "platform.settings.write"},
	Run: func(c *mcpl.Call, in UpdateInput) (Summary, error) {
		if in.Paused == nil || in.ExpectedConfigGeneration < 1 {
			return Summary{}, mcpl.Invalid("Refresh the source and provide its expected configuration generation and pause state.")
		}
		seed, err := seedFromBase64(in.SigningKey, true)
		if err != nil {
			return Summary{}, SafeError(err)
		}
		defer clear(seed)
		r, err := repo(c)
		if err != nil {
			return Summary{}, SafeError(err)
		}
		s, err := seal(c)
		if err != nil {
			return Summary{}, SafeError(err)
		}
		out, err := r.Update(c.Ctx, c.DB, in.ID, acquisition.UpdateCommand{ExpectedConfigGeneration: in.ExpectedConfigGeneration,
			Label: in.Label, Origin: in.Origin, DialIP: in.DialIP, Port: in.Port, CAPEM: in.CAPEM, SigningSeed: seed, Paused: *in.Paused}, s)
		return out, SafeError(err)
	},
}
