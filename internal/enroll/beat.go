package enroll

import (
	"context"
	"errors"
	"log"
	"os"
	"time"
)

// Beat sends one heartbeat with the saved credential.
func (a *Agent) Beat(ctx context.Context, s *saved) error {
	var out map[string]any
	return a.post(ctx, "/v1/servers/heartbeat", s.Credential, map[string]string{"version": a.Version}, &out)
}

// Run enrolls (or loads the saved credential) and beats until ctx ends. A rejected credential
// is removed so the next start needs a fresh token; transient errors just wait for the next beat.
func (a *Agent) Run(ctx context.Context) error {
	s, err := a.Enroll(ctx)
	if err != nil {
		return err
	}
	log.Printf("enroll: server %s, heartbeat every %ds", s.ServerID, s.HeartbeatSeconds)
	t := time.NewTicker(time.Duration(s.HeartbeatSeconds) * time.Second)
	defer t.Stop()
	for {
		if err := a.Beat(ctx, s); errors.Is(err, ErrRejected) {
			_ = os.Remove(a.Cfg.CredentialFile)
			return err
		} else if err != nil {
			log.Printf("enroll: heartbeat: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
