package api

import (
	"log"
	"net"
	"sync"
	"time"

	"shippin.cloud/console/internal/bridge"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/sshca"
)

// bridgeDeps backs devspace access through the Shippin MCP bridge; nil while bridge.enabled is
// false (every bridge route then answers 404).
type bridgeDeps struct {
	cfg    config.Bridge
	store  *bridge.Store
	ca     *sshca.CA
	mu     sync.Mutex
	relays map[string]map[net.Conn]struct{} // open relays per bridge session, closed on revoke
}

func newBridgeDeps(cfg config.Bridge, d *db.DB) *bridgeDeps {
	if !cfg.Enabled {
		return nil
	}
	ca, err := sshca.Load(cfg.CAKeyFile)
	if err != nil {
		log.Printf("bridge: disabled: %v", err)
		return nil
	}
	log.Printf("bridge: on (cert ttl %ds, session %dh)", cfg.CertTTLSeconds, cfg.SessionHours)
	return &bridgeDeps{cfg: cfg, store: &bridge.Store{Pool: d.Pool}, ca: ca, relays: map[string]map[net.Conn]struct{}{}}
}

func (b *bridgeDeps) certTTL() time.Duration {
	return time.Duration(b.cfg.CertTTLSeconds) * time.Second
}

func (b *bridgeDeps) track(session string, c net.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.relays[session] == nil {
		b.relays[session] = map[net.Conn]struct{}{}
	}
	b.relays[session][c] = struct{}{}
}

func (b *bridgeDeps) untrack(session string, c net.Conn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.relays[session], c)
}

// closeAll ends every open relay of a session (revocation applies immediately).
func (b *bridgeDeps) closeAll(session string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for c := range b.relays[session] {
		_ = c.Close()
		n++
	}
	delete(b.relays, session)
	return n
}
