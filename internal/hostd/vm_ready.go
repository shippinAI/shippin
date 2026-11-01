package hostd

import (
	"context"
	"strings"
	"time"
)

// waitInner waits until the Incus inside the tenant VM is ready and the workspaces that were
// running at shutdown (volatile.last_state.power=RUNNING) have been auto-started again.
func waitInner(ctx context.Context, t string) error {
	at := place{t: t, vm: true}
	if _, err := at.do(ctx, "admin", "waitready", "--timeout=90"); err != nil {
		return err
	}
	for i := 0; i < 45; i++ {
		// Every inner project. place.query would pin project=ws and miss pj-* workspaces,
		// and Incus 6.0 rejects all-projects combined with project=.
		if list, err := listInner(ctx, at, 1); err == nil && autostarted(list) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil // report state as is; the operator view shows what is still stopped
}

func autostarted(list []innerInst) bool {
	for _, i := range list {
		// Activation containers are not boot.autostart; they must not hold VM ready.
		if !strings.HasPrefix(i.Name, "w-") {
			continue
		}
		if i.Config["volatile.last_state.power"] == "RUNNING" && i.Status != "Running" {
			return false
		}
	}
	return true
}
