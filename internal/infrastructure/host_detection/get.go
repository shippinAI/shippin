package host_detection

import (
	"errors"
	"strings"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/mcpl"
)

// Get is shippin.infrastructure.host_detection.get: GET /v1/operator/infrastructure/host-detection?address=...
//
// It is a read, a GET taking the address as its query, because it changes nothing: it stores nothing,
// spends no token and sends the server no credentials, only an unauthenticated GET to each kind's version
// path (hv.Detect). So it needs no reason and writes no audit entry, like the add form's connection test.
// It still makes the panel connect to an address a person typed, so it takes the permission to add hosts,
// infra.hosts.write, not infra.read; and it answers with what it classified, never the raw replies.
//
// A server that does not answer, or answers as no kind the panel drives, is a result (outcome
// "unreachable" or "unrecognised"), not an error. Only an address it will not try is refused, with 400.
var Get = mcpl.Def[Query, hv.Detection]{
	Spec: mcpl.Spec{Summary: "Find out from an address alone whether an Incus or Proxmox VE server answers there, and show its certificate.", Access: mcpl.Staff, Perm: "infra.hosts.write", Singleton: true},
	Run: func(c *mcpl.Call, in Query) (hv.Detection, error) {
		if _, _, err := hv.ParseAddress(in.Address); err != nil {
			return hv.Detection{}, mcpl.Invalid(plain(err))
		}
		p, err := mcpl.Need[Prober](c)
		if err != nil {
			return hv.Detection{}, err
		}
		d, err := p.Detect(c.Ctx, in.Address)
		if errors.Is(err, hv.ErrBadAddress) {
			return hv.Detection{}, mcpl.Invalid(plain(err))
		}
		return d, err
	},
}

// plain is a refusal from hv as a sentence: "hv: bad address: leave the path out" is "Leave the path out."
func plain(err error) string {
	msg := err.Error()
	if _, rest, ok := strings.Cut(msg, hv.ErrBadAddress.Error()+": "); ok {
		msg = rest
	}
	if msg == "" {
		return "That address can't be checked."
	}
	return strings.ToUpper(msg[:1]) + msg[1:] + "."
}
