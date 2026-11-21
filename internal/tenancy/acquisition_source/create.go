package acquisition_source

import (
	"shippin.cloud/console/internal/acquisition"
	"shippin.cloud/console/internal/mcpl"
)

type CreateInput struct {
	Label         string `json:"label"`
	Origin        string `json:"origin"`
	DialIP        string `json:"dial_ip"`
	Port          int    `json:"port"`
	CAPEM         string `json:"ca_pem"`
	SourceTenant  string `json:"source_tenant"`
	SourceProject string `json:"source_project"`
	Issuer        string `json:"issuer"`
	Audience      string `json:"audience"`
	SigningKey    string `json:"signing_key"`
}

var Create = mcpl.Def[CreateInput, Summary]{
	Spec: mcpl.Spec{Summary: "Register a private acquisition source with pinned TLS and a write-only signer key.", Access: mcpl.Staff, Perm: "platform.settings.write"},
	Run: func(c *mcpl.Call, in CreateInput) (Summary, error) {
		seed, err := seedFromBase64(in.SigningKey, false)
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
		out, err := r.Create(c.Ctx, c.DB, acquisition.CreateCommand{Label: in.Label, Origin: in.Origin, DialIP: in.DialIP, Port: in.Port,
			CAPEM: in.CAPEM, SourceTenant: in.SourceTenant, SourceProject: in.SourceProject, Issuer: in.Issuer, Audience: in.Audience, SigningSeed: seed}, s)
		return out, SafeError(err)
	},
}
