package api

import (
	"context"
	"encoding/json"
	"errors"

	"shippin.cloud/console/internal/deployments"
)

var errDeploymentStoragePoolUnavailable = errors.New("deployment storage pool is not in the assigned target inventory")
var errDeploymentNetworkUnavailable = errors.New("deployment network is not in the supported assigned target inventory")

// validateDeploymentTargetConfig rechecks selected Incus resources against
// live provider inventory before accepting a new desired generation. The UI
// is not an authority for target assignment or provider state.
func (a *API) validateDeploymentTargetConfig(ctx context.Context, tenantID, systemID string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var targetConfig deployments.TargetConfig
	if err := json.Unmarshal(raw, &targetConfig); err != nil ||
		(targetConfig.Storage == nil && targetConfig.Network == nil) ||
		(targetConfig.Storage != nil && targetConfig.Storage.Pool == "" && targetConfig.Network == nil) ||
		(targetConfig.Network != nil && targetConfig.Network.Name == "" && targetConfig.Storage == nil) {
		// The deployment store performs strict shape and runtime validation. Do
		// not open a provider connection for malformed or resource-free settings.
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
	if targetConfig.Storage != nil {
		pools, err := driver.Storage(ctx)
		if err != nil {
			return errors.New("deployment target storage inventory is unavailable")
		}
		found := false
		for _, pool := range pools {
			if pool.Name == targetConfig.Storage.Pool {
				found = true
				break
			}
		}
		if !found {
			return errDeploymentStoragePoolUnavailable
		}
	}
	if targetConfig.Network != nil {
		networks, err := driver.Networks(ctx)
		if err != nil {
			return errors.New("deployment target network inventory is unavailable")
		}
		found := false
		for _, network := range networks {
			if network.Name == targetConfig.Network.Name && (network.Type == "bridge" || network.Type == "ovn") {
				found = true
				break
			}
		}
		if !found {
			return errDeploymentNetworkUnavailable
		}
	}
	return nil
}
