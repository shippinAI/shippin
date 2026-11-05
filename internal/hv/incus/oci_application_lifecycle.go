package incus

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/hv"
)

// InspectOCIApplication performs provider read-back for an immutable Shippin
// OCI application identity without changing guest state. It is suitable for
// recovery after an ambiguous create response; callers must still hold the
// current generation and lease before recording the observation.
func (c *Client) InspectOCIApplication(ctx context.Context, spec OCIApplicationSpec) (hv.Guest, bool, error) {
	if _, err := validateOCIApplicationSpec(spec); err != nil {
		return hv.Guest{}, false, err
	}
	health, err := c.Health(ctx)
	if err != nil {
		return hv.Guest{}, false, fmt.Errorf("incus: application provider inspection failed: %w", err)
	}
	if !incusVersionAtLeast(health.Version, 7, 3, 0) {
		return hv.Guest{}, false, fmt.Errorf("incus: OCI application inspection requires server version 7.3.0 or newer (server reports %q)", health.Version)
	}
	instance, err := c.instanceDetails(ctx, spec.Name)
	if errors.Is(err, hv.ErrGuestNotFound) {
		return hv.Guest{}, false, nil
	}
	if err != nil {
		return hv.Guest{}, false, err
	}
	if err := validateOCIApplicationIdentity(instance, spec); err != nil {
		return hv.Guest{}, false, err
	}
	if spec.StoragePool != "" {
		if err := c.checkOCIApplicationVolume(ctx, spec); err != nil {
			return hv.Guest{}, false, err
		}
	}
	return instance.guest(), true, nil
}

// DeleteOCIApplication removes an exact Shippin-owned OCI application
// instance and confirms its absence. The deployment-owned custom volume is
// deliberately retained; deleting application data needs a separate,
// explicitly reviewed and authorized operation. Callers must still perform
// current-generation, job-lease and AgentHub checks immediately before use.
// Incus does not offer a conditional delete by Shippin ownership metadata, so
// this provider primitive must not be treated as a substitute for those gates.
func (c *Client) DeleteOCIApplication(ctx context.Context, spec OCIApplicationSpec) error {
	if _, err := validateOCIApplicationSpec(spec); err != nil {
		return err
	}
	instance, err := c.instanceDetails(ctx, spec.Name)
	if errors.Is(err, hv.ErrGuestNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := validateOCIApplicationPowerIdentity(instance, spec); err != nil {
		return fmt.Errorf("incus: refusing to delete unowned application: %w", err)
	}
	if err := c.ensureOCIApplicationHasNoSnapshots(ctx, spec.Name); err != nil {
		return err
	}
	if !strings.EqualFold(instance.Status, "Stopped") {
		if err := c.StopOCIApplication(ctx, spec); err != nil {
			return fmt.Errorf("incus: stop application before delete: %w", err)
		}
	}
	return c.DeleteOCIApplicationStopped(ctx, spec)
}

// StopOCIApplication changes only the power state of an exact owned instance
// and confirms the stopped state. It does not remove the instance or volume.
// Ownership checks use deployment/generation, image and resource/network/storage
// identity; environment secrets are not needed to stop or remove the workload.
func (c *Client) StopOCIApplication(ctx context.Context, spec OCIApplicationSpec) error {
	if _, err := validateOCIApplicationSpec(spec); err != nil {
		return err
	}
	return c.stopOwnedOCIApplication(ctx, spec)
}

// DeleteOCIApplicationStopped removes an exact owned instance only when it is
// already stopped. It preserves the deployment volume and confirms absence.
// This lets callers put stop and delete behind distinct step intents.
func (c *Client) DeleteOCIApplicationStopped(ctx context.Context, spec OCIApplicationSpec) error {
	if _, err := validateOCIApplicationSpec(spec); err != nil {
		return err
	}
	instance, err := c.instanceDetails(ctx, spec.Name)
	if errors.Is(err, hv.ErrGuestNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := validateOCIApplicationPowerIdentity(instance, spec); err != nil {
		return fmt.Errorf("incus: refusing to delete application after ownership changed: %w", err)
	}
	if !strings.EqualFold(instance.Status, "Stopped") {
		return fmt.Errorf("incus: refusing to delete application in state %q", instance.Status)
	}
	// Incus deletes instance snapshots together with the instance. Recheck after
	// stop and refuse removal when snapshots exist so delete cannot silently
	// discard a recovery point.
	if err := c.ensureOCIApplicationHasNoSnapshots(ctx, spec.Name); err != nil {
		return err
	}

	body, code, err := c.do(ctx, http.MethodDelete, c.path(instancePath(spec.Name)), nil)
	if err != nil {
		return err
	}
	if code != http.StatusNotFound && code != http.StatusOK && code != http.StatusAccepted {
		return fmt.Errorf("incus: delete application HTTP %d: %s", code, clip(body))
	}
	if code != http.StatusNotFound {
		if err := c.wait(ctx, body); err != nil {
			return fmt.Errorf("incus: delete application operation failed: %w", err)
		}
	}
	if _, err := c.instanceDetails(ctx, spec.Name); err == nil {
		return fmt.Errorf("incus: application %s still present after delete", spec.Name)
	} else if !errors.Is(err, hv.ErrGuestNotFound) {
		return fmt.Errorf("incus: couldn't confirm application %s was deleted: %w", spec.Name, err)
	}
	return nil
}

func (c *Client) stopOwnedOCIApplication(ctx context.Context, spec OCIApplicationSpec) error {
	instance, err := c.instanceDetails(ctx, spec.Name)
	if errors.Is(err, hv.ErrGuestNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := validateOCIApplicationPowerIdentity(instance, spec); err != nil {
		return fmt.Errorf("application ownership changed before stop: %w", err)
	}
	if strings.EqualFold(instance.Status, "Stopped") {
		return nil
	}
	if !strings.EqualFold(instance.Status, "Running") {
		return fmt.Errorf("incus: cannot stop OCI application from provider state %q", instance.Status)
	}
	if err := c.putState(ctx, spec.Name, "stop"); err != nil {
		return err
	}
	instance, err = c.instanceDetails(ctx, spec.Name)
	if err != nil {
		return err
	}
	if err := validateOCIApplicationPowerIdentity(instance, spec); err != nil {
		return fmt.Errorf("application ownership changed while stopping: %w", err)
	}
	if !strings.EqualFold(instance.Status, "Stopped") {
		return fmt.Errorf("provider reports application state %q after stop", instance.Status)
	}
	return nil
}

func (c *Client) ensureOCIApplicationHasNoSnapshots(ctx context.Context, name string) error {
	snapshots, err := c.Snapshots(ctx, name)
	if err != nil {
		return fmt.Errorf("incus: inspect application snapshots before delete: %w", err)
	}
	if len(snapshots) != 0 {
		return fmt.Errorf("incus: refusing to delete application with %d snapshots; preserve or remove them explicitly first", len(snapshots))
	}
	return nil
}
