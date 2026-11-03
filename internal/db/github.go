package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// GitHubInstallation is a Shippin GitHub App installation bound to one tenant (018_github.sql).
type GitHubInstallation struct {
	ID            int64  `json:"id"`
	TenantID      string `json:"tenant_id"`
	AccountLogin  string `json:"account_login"`
	AccountType   string `json:"account_type"`
	RepoSelection string `json:"repo_selection"`
	Suspended     bool   `json:"suspended"`
}

// ErrInstallationTaken: the installation is already bound to another tenant.
var ErrInstallationTaken = errors.New("installation belongs to another tenant")

// BindInstallation records installation inst for tenant (re-binding a deleted one is allowed;
// moving a live one to another tenant is not).
func (d *DB) BindInstallation(ctx context.Context, in GitHubInstallation, account string) error {
	var acct any
	if account != "" {
		acct = account
	}
	tag, err := d.Pool.Exec(ctx, `INSERT INTO github_installations(id, tenant_id, account_login, account_type, repo_selection, created_by)
	  VALUES ($1,$2,$3,$4,$5,$6)
	  ON CONFLICT (id) DO UPDATE SET account_login=EXCLUDED.account_login, account_type=EXCLUDED.account_type,
	    repo_selection=EXCLUDED.repo_selection, tenant_id=EXCLUDED.tenant_id, deleted_at=NULL, updated_at=now()
	  WHERE github_installations.tenant_id=EXCLUDED.tenant_id OR github_installations.deleted_at IS NOT NULL`,
		in.ID, in.TenantID, in.AccountLogin, in.AccountType, in.RepoSelection, acct)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrInstallationTaken
	}
	return err
}

// Installations lists a tenant's live installations.
func (d *DB) Installations(ctx context.Context, tenant string) ([]GitHubInstallation, error) {
	rows, err := d.Pool.Query(ctx, `SELECT id, tenant_id::text, account_login, account_type, repo_selection, suspended_at IS NOT NULL
	  FROM github_installations WHERE tenant_id=$1 AND deleted_at IS NULL ORDER BY created_at`, tenant)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (GitHubInstallation, error) {
		var g GitHubInstallation
		return g, r.Scan(&g.ID, &g.TenantID, &g.AccountLogin, &g.AccountType, &g.RepoSelection, &g.Suspended)
	})
}

// AllInstallationIDs lists every live installation id (polling).
func (d *DB) AllInstallationIDs(ctx context.Context) ([]int64, error) {
	rows, err := d.Pool.Query(ctx, `SELECT id FROM github_installations WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// SetInstallationState applies a webhook or poll result: deleted, suspended or active.
func (d *DB) SetInstallationState(ctx context.Context, inst int64, state string) error {
	q := map[string]string{
		"deleted":   `UPDATE github_installations SET deleted_at=now(), updated_at=now() WHERE id=$1`,
		"suspended": `UPDATE github_installations SET suspended_at=now(), updated_at=now() WHERE id=$1`,
		"active":    `UPDATE github_installations SET suspended_at=NULL, updated_at=now() WHERE id=$1`,
	}[state]
	if q == "" {
		return errors.New("unknown installation state")
	}
	if _, err := d.Pool.Exec(ctx, q, inst); err != nil {
		return err
	}
	if state == "active" { // repo access is re-derived by SyncRepoAccess (poll)
		return nil
	}
	_, err := d.Pool.Exec(ctx, `UPDATE project_sources SET status='revoked', updated_at=now() WHERE installation_id=$1`, inst)
	return err
}

// SyncRepoAccess sets each source of installation inst to ok or revoked by whether its repo is
// among the repos GitHub currently lists for the installation.
func (d *DB) SyncRepoAccess(ctx context.Context, inst int64, repoIDs []int64) error {
	_, err := d.Pool.Exec(ctx, `UPDATE project_sources SET updated_at=now(),
	  status = CASE WHEN repo_id = ANY($2) THEN 'ok' ELSE 'revoked' END WHERE installation_id=$1`, inst, repoIDs)
	return err
}

// RecordDelivery stores a webhook delivery id; false means it was seen before (replay).
func (d *DB) RecordDelivery(ctx context.Context, id string) (bool, error) {
	tag, err := d.Pool.Exec(ctx, `INSERT INTO github_deliveries(id) VALUES ($1) ON CONFLICT DO NOTHING`, id)
	return err == nil && tag.RowsAffected() == 1, err
}

// PruneDeliveries drops delivery ids older than 7 days.
func (d *DB) PruneDeliveries(ctx context.Context) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM github_deliveries WHERE received_at < now() - interval '7 days'`)
	return err
}
