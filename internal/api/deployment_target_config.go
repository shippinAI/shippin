package api

import (
	"context"
	"encoding/json"
	"errors"

	"shippin.cloud/console/internal/deployments"
)

var errDeploymentStoragePoolUnavailable = errors.New("deployment storage pool is not in the assigned target inventory")

// validateDeploymentStorageTarget rechecks the selected pool against the live
// provider inventory before accepting a new desired generation. The UI is not
// an authority for target assignment or provider state.
func (a *API) validateDeploymentStorageTarget(ctx context.Context, tenantID, systemID string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var targetConfig deployments.TargetConfig
	if err := json.Unmarshal(raw, &targetConfig); err != nil || targetConfig.Storage == nil || targetConfig.Storage.Pool == "" {
		// The deployment store performs strict shape and runtime validation. Do
		// not open a provider connection for malformed or storage-free settings.
		return nil
	}

	targets, err := a.deploymentStore().Targets(ctx, tenantID)
	if err != nil {
		return err
	}
	var assigned *deployments.Target
	for i := range targets {
		if targets[i].ID == systemID {
			assigned = &targets[i]
			break
		}
	}
	if assigned == nil || assigned.Provider != deployments.ProviderIncus || assigned.State != "active" {
		return deployments.ErrTargetUnavailable
	}

	driver, err := a.driver(ctx, assigned.ID)
	if err != nil || driver == nil || driver.Kind() != string(deployments.ProviderIncus) {
		return errors.New("deployment target storage inventory is unavailable")
	}
	pools, err := driver.Storage(ctx)
	if err != nil {
		return errors.New("deployment target storage inventory is unavailable")
	}
	for _, pool := range pools {
		if pool.Name == targetConfig.Storage.Pool {
			return nil
		}
	}
	return errDeploymentStoragePoolUnavailable
}
