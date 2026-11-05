package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/catalog"
	"shippin.cloud/console/internal/deployments"
	"shippin.cloud/console/internal/vaultix"
)

var errDeploymentSecretUnavailable = errors.New("deployment secret resolver is unavailable")

// revealTenantDeploymentSecret resolves one Vaultix key from the project owned
// by tenantID. Project identity is derived from installation configuration;
// it is never accepted from a catalog or tenant-supplied secret reference.
// The caller must already hold a current generation and worker lease.
func (a *API) revealTenantDeploymentSecret(ctx context.Context, tenantID, deploymentID string,
	generation int64, actor, reference, jobID, leaseOwner string) (string, error) {
	if _, err := uuid.Parse(tenantID); err != nil {
		return "", errDeploymentSecretUnavailable
	}
	if _, err := uuid.Parse(deploymentID); err != nil || generation < 1 || actor == "" || len(actor) > 255 ||
		strings.TrimSpace(actor) != actor || strings.ContainsAny(actor, "\r\n\x00") {
		return "", errDeploymentSecretUnavailable
	}
	if _, err := uuid.Parse(jobID); err != nil || strings.TrimSpace(leaseOwner) == "" || len(leaseOwner) > 255 {
		return "", errDeploymentSecretUnavailable
	}
	parsed, err := catalog.ParseVaultixReference(reference)
	if err != nil {
		return "", err
	}
	if a.cfg == nil || a.db == nil || a.n2.vx == nil || a.n2.vx.c == nil || !a.cfg.Vaultix.Enabled || !a.cfg.Vaultix.HasEnv(parsed.Environment) {
		return "", errDeploymentSecretUnavailable
	}
	deployment, err := a.deploymentStore().Get(ctx, tenantID, deploymentID)
	if err != nil || deployment.Generation != generation || deployment.TargetProvider != deployments.ProviderIncus {
		return "", errDeploymentSecretUnavailable
	}
	target, err := a.visibleEndpoint(ctx, tenantID, deployment.TargetSystemID)
	if err != nil || target.Kind != string(deployments.ProviderIncus) || !target.AllowTenantDeploymentSecrets {
		return "", errDeploymentSecretUnavailable
	}
	projectSlug := a.cfg.Vaultix.ProjectPrefix + tenantID
	projectID, err := a.n2.vx.c.ProjectBySlug(ctx, projectSlug)
	if err != nil || projectID == "" {
		return "", errDeploymentSecretUnavailable
	}
	if err := a.auditDeploymentSecretReveal(ctx, tenantID, deploymentID, generation, actor, jobID, leaseOwner); err != nil {
		return "", errDeploymentSecretUnavailable
	}
	_, value, err := a.n2.vx.c.Reveal(ctx, vaultix.Scope{Project: projectID,
		Env: parsed.Environment, Path: parsed.Path}, parsed.Key)
	if err != nil {
		return "", errDeploymentSecretUnavailable
	}
	return value, nil
}

// auditDeploymentSecretReveal binds the secret read to the current desired
// generation and live deployment job lease in the same transaction as its
// audit record. The Vaultix call follows only after this fence succeeds.
func (a *API) auditDeploymentSecretReveal(ctx context.Context, tenantID, deploymentID string,
	generation int64, actor, jobID, leaseOwner string) error {
	tx, err := a.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var leasedJob string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM jobs WHERE id=$1 AND tenant_id=$2
		AND kind='deployment.reconcile' AND target_type='deployment_generation'
		AND target_id=$3 AND status='running' AND lease_owner=$4 AND lease_until>now() FOR UPDATE`,
		jobID, tenantID, deploymentID+":"+fmt.Sprint(generation), leaseOwner).Scan(&leasedJob); err != nil {
		return err
	}
	var current int64
	if err := tx.QueryRow(ctx, `SELECT desired_generation FROM shippin_deployments
		WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, deploymentID, tenantID).Scan(&current); err != nil {
		return err
	}
	if current != generation {
		return errors.New("deployment generation changed")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_log(tenant_id,actor,actor_name,action,detail)
		VALUES($1,$2,'deployment worker','deployment.secret.reveal',$3)`, tenantID, actor,
		fmt.Sprintf("deployment=%s generation=%d", deploymentID, generation)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
