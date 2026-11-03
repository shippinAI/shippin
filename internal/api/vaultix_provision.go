package api

import (
	"context"
	"errors"
	"log"
	"time"

	"shippin.cloud/console/internal/vaultix"
)

// ensureVaultixProject makes the tenant's Vaultix project (<prefix><tenant uuid>, the console
// identity becomes its admin) and its environments if missing, and returns its id.
func (a *API) ensureVaultixProject(ctx context.Context, tenant string) (string, error) {
	vx := a.n2.vx
	if vx == nil {
		return "", errors.New("vaultix disabled")
	}
	slug := vx.cfg.ProjectPrefix + tenant
	pid, err := vx.c.ProjectBySlug(ctx, slug)
	if errors.Is(err, vaultix.ErrNotFound) || (err == nil && pid == "") {
		if pid, err = vx.c.CreateProject(ctx, "Tenant "+tenant, slug); err != nil {
			return "", err
		}
		log.Printf("vaultix: created project %s", slug)
	} else if err != nil {
		return "", err
	}
	return pid, vx.c.EnsureEnvs(ctx, pid, vx.cfg.Envs)
}

// ensureVaultixWorkspace provisions the tenant project and the workspace's connect folder.
// Best effort: it runs in the background on tenant/workspace create; Connect retries it lazily.
func (a *API) ensureVaultixWorkspace(tenant, ws string) {
	if a.n2.vx == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pid, err := a.ensureVaultixProject(ctx, tenant)
	if err == nil && ws != "" {
		err = a.n2.vx.c.EnsureFolder(ctx, pid, a.cfg.Connect.VaultixEnv, a.cfg.Connect.Path(ws))
	}
	if err != nil {
		log.Printf("vaultix: provision tenant %s workspace %s: %v", tenant, ws, err)
	}
}
