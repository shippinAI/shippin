package hv

import (
	"context"
	"errors"
	"fmt"

	"shippin.cloud/console/internal/approval"
)

// ActionLifecycle is the approval action for start, stop, shutdown, reboot, and snapshot.
const ActionLifecycle = "hv.lifecycle"

// ActionAttach is the approval action for adding or detaching a hypervisor API. It changes what
// the console can reach, not any guest. A policy with no rule for it uses ActionLifecycle.
const ActionAttach = "hv.attach"

// ErrDenied means the approval policy does not allow this subject.
var ErrDenied = errors.New("hv: approval policy denied this action")

// ErrWaiting means the factory board has not approved the card.
var ErrWaiting = errors.New("hv: waiting on the factory board")

// Board is the slice of the agent hub the guard needs.
type Board interface {
	Status(ctx context.Context, org, key string) (string, error)
}

// Guard checks policy and, when the rule asks, the board. Drivers do not check this themselves.
type Guard struct {
	Policy  approval.Policy
	Subject approval.Subject
	Host    string
	Org     string
	Board   Board
}

// Allow returns nil when the subject may perform verb on guest.
func (g Guard) Allow(ctx context.Context, verb, guest string) error {
	if !g.Policy.Allows(g.Subject, ActionLifecycle, g.Host) {
		return ErrDenied
	}
	if !g.Policy.NeedsBoard(ActionLifecycle, g.Host) {
		return nil
	}
	if g.Board == nil {
		return fmt.Errorf("%w: no board configured", ErrWaiting)
	}
	key := fmt.Sprintf("%s:%s:%s:%s", ActionLifecycle, g.Host, guest, verb)
	st, err := g.Board.Status(ctx, g.Org, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWaiting, err)
	}
	if st != "approved" {
		return fmt.Errorf("%w: card %s is %s", ErrWaiting, key, st)
	}
	return nil
}

// Run allows verb, then calls fn. fn must return the guest the hypervisor reported.
func (g Guard) Run(ctx context.Context, verb, guest string, fn func() (Guest, error)) (Guest, error) {
	if err := g.Allow(ctx, verb, guest); err != nil {
		return Guest{}, err
	}
	return fn()
}
