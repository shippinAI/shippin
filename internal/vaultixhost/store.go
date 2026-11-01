// Package vaultixhost is Vaultix as a hosted, multi-tenant service: every signed-in Shippin
// person (Nomen subject) gets their own Vaultix organization and workspace, provisioned on
// first use, and reaches it only through the console session or their own keyring identity.
package vaultixhost

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Account is one person's Vaultix tenancy. ConsoleSeal is the sealed console client secret.
type Account struct {
	Sub, Email, NomenOrg, Org, Project string
	ConsoleID, ConsoleCID              string
	ConsoleSeal                        []byte
	KeyringID, KeyringCID              string
}

// Store persists accounts in the console database.
type Store struct{ Pool *pgxpool.Pool }

const cols = `sub, email, nomen_org, vx_org, project_id, console_id, console_cid, console_seal, keyring_id, keyring_cid`

// Get returns the account of sub, or (nil, nil) if it has none yet.
func (s *Store) Get(ctx context.Context, sub string) (*Account, error) {
	a := &Account{}
	err := s.Pool.QueryRow(ctx, `SELECT `+cols+` FROM vaultix_accounts WHERE sub=$1`, sub).Scan(&a.Sub, &a.Email,
		&a.NomenOrg, &a.Org, &a.Project, &a.ConsoleID, &a.ConsoleCID, &a.ConsoleSeal, &a.KeyringID, &a.KeyringCID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// Insert records a new account; a concurrent first use loses quietly and re-reads.
func (s *Store) Insert(ctx context.Context, a *Account) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO vaultix_accounts (`+cols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (sub) DO NOTHING`, a.Sub, a.Email, a.NomenOrg, a.Org, a.Project, a.ConsoleID, a.ConsoleCID,
		a.ConsoleSeal, a.KeyringID, a.KeyringCID)
	return err
}

// SetKeyring records the keyring identity (ids only).
func (s *Store) SetKeyring(ctx context.Context, sub, id, cid string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE vaultix_accounts SET keyring_id=$2, keyring_cid=$3 WHERE sub=$1`, sub, id, cid)
	return err
}
