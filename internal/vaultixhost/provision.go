package vaultixhost

import (
	"context"
	"sync"
	"time"

	"shippin.cloud/console/internal/vaultix"
)

// Service provisions and opens accounts. Envs, names and URLs come from config.
type Service struct {
	Store     *Store
	Op        *vaultix.Operator
	Seal      *Sealer
	Base      string        // Vaultix API base the console reaches
	PublicURL string        // Vaultix URL a person's keyring uses
	Envs      []string      // environments every workspace gets
	Timeout   time.Duration // per Vaultix call
	mu        sync.Mutex    // one provisioning at a time per replica (first use is rare)
}

// Person is the signed-in Nomen identity.
type Person struct{ Sub, Email, Org string }

// Ensure returns p's account, creating the Vaultix organization, console identity and
// workspace on first use.
func (s *Service) Ensure(ctx context.Context, p Person) (*Account, error) {
	if a, err := s.Store.Get(ctx, p.Sub); a != nil || err != nil {
		return a, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, err := s.Store.Get(ctx, p.Sub); a != nil || err != nil {
		return a, err
	}
	org, err := s.Op.CreateOrg(ctx, "vx-"+p.Sub)
	if err != nil {
		return nil, err
	}
	id, cid, err := s.Op.CreateIdentity(ctx, org, "console")
	if err != nil {
		return nil, err
	}
	sec, err := s.Op.MintSecret(ctx, org, id, "Shippin console")
	if err != nil {
		return nil, err
	}
	c := vaultix.New(s.Base, cid, sec, s.Timeout)
	pid, err := c.CreateProject(ctx, "Workspace", "ws-"+p.Sub)
	if err != nil {
		return nil, err
	}
	if err := c.EnsureEnvs(ctx, pid, s.Envs); err != nil {
		return nil, err
	}
	box, err := s.Seal.Seal(sec, p.Sub)
	if err != nil {
		return nil, err
	}
	a := &Account{Sub: p.Sub, Email: p.Email, NomenOrg: p.Org, Org: org, Project: pid, ConsoleID: id, ConsoleCID: cid, ConsoleSeal: box}
	if err := s.Store.Insert(ctx, a); err != nil {
		return nil, err
	}
	return s.Store.Get(ctx, p.Sub)
}

// Client is a Vaultix client acting as a's console identity (scoped to a's organization only).
func (s *Service) Client(a *Account) (*vaultix.Client, error) {
	sec, err := s.Seal.Open(a.ConsoleSeal, a.Sub)
	if err != nil {
		return nil, err
	}
	return vaultix.New(s.Base, a.ConsoleCID, sec, s.Timeout), nil
}

// KeyringSecret mints a fresh client secret for a's keyring identity (created on first use).
// The secret is returned once and not stored.
func (s *Service) KeyringSecret(ctx context.Context, a *Account, label string) (cid, secret string, err error) {
	if a.KeyringID == "" {
		id, c, err := s.Op.CreateIdentity(ctx, a.Org, "keyring")
		if err != nil {
			return "", "", err
		}
		cl, err := s.Client(a)
		if err != nil {
			return "", "", err
		}
		if err := cl.AddIdentity(ctx, a.Project, id, "member"); err != nil {
			return "", "", err
		}
		if err := s.Store.SetKeyring(ctx, a.Sub, id, c); err != nil {
			return "", "", err
		}
		a.KeyringID, a.KeyringCID = id, c
	}
	secret, err = s.Op.MintSecret(ctx, a.Org, a.KeyringID, label)
	return a.KeyringCID, secret, err
}
