package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/catalog"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/vaultix"
)

var errDeploymentSecretUnavailable = errors.New("deployment secret resolver is unavailable")

// revealTenantDeploymentSecret resolves one Vaultix key from the project owned
// by tenantID. Project identity is derived from installation configuration;
// it is never accepted from a catalog or tenant-supplied secret reference.
// The caller must already hold a current generation and worker lease.
func (a *API) revealTenantDeploymentSecret(ctx context.Context, tenantID, deploymentID string,
	generation int64, actor, reference string) (string, error) {
	if _, err := uuid.Parse(tenantID); err != nil {
		return "", errDeploymentSecretUnavailable
	}
	if _, err := uuid.Parse(deploymentID); err != nil || generation < 1 || actor == "" || len(actor) > 255 ||
		strings.TrimSpace(actor) != actor || strings.ContainsAny(actor, "\r\n\x00") {
		return "", errDeploymentSecretUnavailable
	}
	parsed, err := catalog.ParseVaultixReference(reference)
	if err != nil {
		return "", err
	}
	if a.cfg == nil || a.db == nil || a.n2.vx == nil || a.n2.vx.c == nil || !a.cfg.Vaultix.Enabled || !a.cfg.Vaultix.HasEnv(parsed.Environment) {
		return "", errDeploymentSecretUnavailable
	}
	projectSlug := a.cfg.Vaultix.ProjectPrefix + tenantID
	projectID, err := a.n2.vx.c.ProjectBySlug(ctx, projectSlug)
	if err != nil || projectID == "" {
		return "", errDeploymentSecretUnavailable
	}
	if err := a.db.AddAudit(ctx, db.AuditEntry{TenantID: tenantID, Actor: actor,
		ActorName: "deployment worker", Action: "deployment.secret.reveal",
		Detail: fmt.Sprintf("deployment=%s generation=%d", deploymentID, generation)}); err != nil {
		return "", errDeploymentSecretUnavailable
	}
	_, value, err := a.n2.vx.c.Reveal(ctx, vaultix.Scope{Project: projectID,
		Env: parsed.Environment, Path: parsed.Path}, parsed.Key)
	if err != nil {
		return "", errDeploymentSecretUnavailable
	}
	return value, nil
}
