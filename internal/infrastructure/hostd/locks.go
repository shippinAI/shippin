package hostd

import "sync"

// wsLocks serialises operations on one workspace (create, actions, teardown) so concurrent
// requests can't interleave edge route / ACL / port updates. Different workspaces run in parallel.
var wsLocks sync.Map

func lockWorkspace(w string) func() {
	m, _ := wsLocks.LoadOrStore(w, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
