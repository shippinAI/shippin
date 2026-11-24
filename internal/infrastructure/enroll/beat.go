package enroll

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"time"
)

// Beat sends one heartbeat with the saved credential.
func (a *Agent) Beat(ctx context.Context, s *saved) error {
	var out struct {
		Command *Command `json:"command,omitempty"`
	}
	if err := a.post(ctx, "/v1/servers/heartbeat", s.Credential, map[string]string{"version": a.Version}, &out); err != nil {
		return err
	}
	if out.Command == nil {
		return nil
	}
	if !out.Command.ExpiresAt.After(time.Now()) {
		return errors.New("console delivered an expired host command")
	}
	if a.HandleCommand == nil {
		return errors.New("console delivered a command but host has no configured command handler")
	}
	result, err := a.HandleCommand(ctx, *out.Command)
	if err != nil {
		return err
	}
	body, err := json.Marshal(result)
	if err != nil {
		return errors.New("host command result could not be encoded")
	}
	return a.post(ctx, "/v1/servers/commands/"+out.Command.ID+"/result", s.Credential, json.RawMessage(body), &struct{}{})
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
