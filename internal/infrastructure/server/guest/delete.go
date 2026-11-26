package guest

import (
	"context"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/server"
	"shippin.cloud/console/internal/mcpl"
)

// DeleteInput names the guest and repeats its id, so a stale page or a replayed request cannot remove
// a different one.
type DeleteInput struct {
	ByID
	// Confirm is the guest's id, repeated.
	Confirm string `json:"confirm"`
	// ConfirmSelf must be true to delete the guest this panel itself runs on.
	ConfirmSelf bool `json:"confirm_self,omitempty"`
}

// Delete is shippin.infrastructure.server.guest.delete: DELETE /v1/operator/infrastructure/servers/{serverId}/guests/{guestId}.
var Delete = mcpl.Def[DeleteInput, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Delete a guest from a server with its disks; this cannot be undone.", Access: mcpl.Staff, Perm: "infra.guests.manage"},
	Run: func(c *mcpl.Call, in DeleteInput) (mcpl.None, error) {
		if in.Confirm != in.ID {
			return mcpl.None{}, mcpl.Invalid("Confirm what you are removing: send the guest's id, " + in.ID + ", as confirm.")
		}
		h, _, d, err := server.Open(c, in.Server)
		if err != nil {
			return mcpl.None{}, err
		}
		if _, err := current(c, d, in.ID, "delete", in.ConfirmSelf); err != nil {
			return mcpl.None{}, err
		}
		m, ok := d.(hv.GuestManager)
		if !ok {
			return mcpl.None{}, mcpl.Conflict("This kind of server does not remove guests.")
		}
		_, err = h.Track(c.Ctx, operation(c, in.Server, in.ID, "delete"), func(ctx context.Context) (hv.Guest, error) {
			return hv.Guest{}, m.DeleteGuest(ctx, in.ID)
		})
		return mcpl.None{}, server.Problem(err)
	},
}
