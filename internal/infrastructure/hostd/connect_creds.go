package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// SpoolReq is one writer request: "set" a pasted token or "forget" the provider entirely.
type SpoolReq struct {
	Op        string     `json:"op"`
	Token     string     `json:"token,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// ConnectStatus is the writer's report: per provider, connected or not, how, and until when.
type ConnectStatus struct {
	CheckedAt time.Time        `json:"checked_at"`
	Providers []ProviderStatus `json:"providers"`
}

type ProviderStatus struct {
	Provider  string     `json:"provider"`
	Connected bool       `json:"connected"`
	Method    string     `json:"method,omitempty"` // browser | token
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// PutToken validates a pasted token against the provider's pattern and hands it to the writer.
func (s *Server) PutToken(ctx context.Context, t, w, id string, req SpoolReq) error {
	p, err := s.connectProvider(id)
	if err != nil {
		return err
	}
	if !regexp.MustCompile(p.TokenPattern).MatchString(req.Token) {
		return fmt.Errorf("that doesn't look like a %s token", id)
	}
	req.Op = "set"
	return s.toWriter(ctx, t, w, id, req)
}

// Disconnect asks the writer to sign the CLI out and drop any stored token.
func (s *Server) Disconnect(ctx context.Context, t, w, id string) error {
	if _, err := s.connectProvider(id); err != nil {
		return err
	}
	if v, ok := logins.LoadAndDelete(w + "/" + id); ok {
		v.(*loginSession).cancel()
	}
	return s.toWriter(ctx, t, w, id, SpoolReq{Op: "forget"})
}

func (s *Server) toWriter(ctx context.Context, t, w, id string, req SpoolReq) error {
	at, err := s.placeOf(ctx, t, w)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(req)
	if err := s.spool(ctx, at, Instance(w), id+".json", string(body)); err != nil {
		return fmt.Errorf("credential not handed to the workspace")
	}
	return s.kickWriter(ctx, at, Instance(w))
}

// ConnectStatusOf reads the writer's bounded status report; unknown providers are dropped.
func (s *Server) ConnectStatusOf(ctx context.Context, t, w string) (*ConnectStatus, error) {
	c := s.cfg.Connect
	if !c.enabled() {
		return nil, fmt.Errorf("connect accounts is not configured")
	}
	at, err := s.placeOf(ctx, t, w)
	if err != nil {
		return nil, err
	}
	out, err := at.do(ctx, "exec", Instance(w), "--", "head", "-c", fmt.Sprint(c.StatusMax+1), c.StatusPath)
	if err != nil {
		return &ConnectStatus{Providers: []ProviderStatus{}}, nil // writer not started yet
	}
	if len(out) > c.StatusMax {
		return nil, fmt.Errorf("connect status too large")
	}
	var st ConnectStatus
	if json.Unmarshal([]byte(out), &st) != nil {
		return nil, fmt.Errorf("connect status malformed")
	}
	keep := []ProviderStatus{}
	for _, p := range st.Providers {
		if _, ok := c.Providers[p.Provider]; ok {
			keep = append(keep, p)
		}
	}
	st.Providers = keep
	return &st, nil
}
