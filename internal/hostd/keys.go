package hostd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// storeKey generates a 32-byte tenant ZFS key and stores it in the sops key store
// via stdin (never on argv, never printed). Returns the hex key for zfs create.
func (s *Server) storeKey(ctx context.Context, tenant string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	k := hex.EncodeToString(b)
	path := fmt.Sprintf(`["%s"]`, tenant)
	if _, err := run(ctx, `"`+k+`"`, "sops", "set", "--value-stdin", s.cfg.KeyStore, path); err != nil {
		return "", fmt.Errorf("store tenant key: %w", err)
	}
	return k, nil
}

// loadKey reads a stored tenant key (used to re-load after reboot).
func (s *Server) loadKey(ctx context.Context, tenant string) (string, error) {
	return run(ctx, "", "sops", "decrypt", "--extract", fmt.Sprintf(`["%s"]`, tenant), s.cfg.KeyStore)
}
