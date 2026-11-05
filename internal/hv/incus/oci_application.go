package incus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/hv"
)

// OCIApplicationSpec describes the deliberately small, stateless Incus OCI
// workload primitive. It does not configure credentials, persistent data,
// network exposure, health checks, or upgrades; catalog deployments must not
// use it until those lifecycle contracts are implemented as well.
type OCIApplicationSpec struct {
	Name           string
	DeploymentID   string
	Generation     int64
	ImageReference string
	MemoryMiB      int
	Cores          int
	Start          bool
}

type ociImageReference struct {
	Server string
	Alias  string
}

var ociDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// parseOCIApplicationReference accepts a fully qualified registry/repository
// reference with an immutable OCI manifest digest. Registry credentials are
// intentionally out of scope for this anonymous-registry primitive.
func parseOCIApplicationReference(reference string) (ociImageReference, error) {
	if len(reference) > 1024 || strings.ContainsAny(reference, "\r\n\x00") {
		return ociImageReference{}, errors.New("incus: OCI image reference is invalid")
	}
	registryAndPath, digest, ok := strings.Cut(reference, "@")
	if !ok || !ociDigestPattern.MatchString(digest) || strings.Contains(digest, "@") {
		return ociImageReference{}, errors.New("incus: OCI image reference must use a sha256 manifest digest")
	}
	registry, repository, ok := strings.Cut(registryAndPath, "/")
	if !ok || registry == "" || repository == "" || strings.Contains(repository, "\\") || strings.Contains(repository, "..") {
		return ociImageReference{}, errors.New("incus: OCI image reference must include a registry and repository")
	}
	parsed, err := url.Parse("https://" + registry)
	if err != nil || parsed.Host != registry || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ociImageReference{}, errors.New("incus: OCI registry must be a host name with an optional port")
	}
	for _, component := range strings.Split(repository, "/") {
		if component == "" || component == "." || component == ".." || strings.ContainsAny(component, " :?#%") {
			return ociImageReference{}, errors.New("incus: OCI repository path is invalid")
		}
	}
	return ociImageReference{Server: "https://" + registry, Alias: repository + "@" + digest}, nil
}

// CreateOCIApplication creates one digest-pinned Incus application container,
// waits for the provider operation, and reads the resulting guest back. Callers
// must perform their own current-generation, lease and AgentHub checks before
// invoking this provider mutation.
func (c *Client) CreateOCIApplication(ctx context.Context, spec OCIApplicationSpec) (hv.Guest, error) {
	if !guestName.MatchString(spec.Name) {
		return hv.Guest{}, errors.New("incus: application name starts with a letter and uses letters, digits and dashes")
	}
	if _, err := uuid.Parse(spec.DeploymentID); err != nil || spec.Generation < 1 {
		return hv.Guest{}, errors.New("incus: application requires a deployment UUID and positive generation")
	}
	image, err := parseOCIApplicationReference(spec.ImageReference)
	if err != nil {
		return hv.Guest{}, err
	}
	if spec.Cores < 1 || spec.Cores > 32 || spec.MemoryMiB < 128 || spec.MemoryMiB > 65536 {
		return hv.Guest{}, errors.New("incus: application resources must be 1-32 cores and 128-65536 MiB memory")
	}

	health, err := c.Health(ctx)
	if err != nil {
		return hv.Guest{}, fmt.Errorf("incus: application provider preflight failed: %w", err)
	}
	if !incusVersionAtLeast(health.Version, 7, 3, 0) {
		return hv.Guest{}, fmt.Errorf("incus: OCI application pulls require server version 7.3.0 or newer (server reports %q)", health.Version)
	}

	instance, err := c.instanceDetails(ctx, spec.Name)
	if err == nil {
		if err := validateOCIApplicationIdentity(instance, spec); err != nil {
			return hv.Guest{}, err
		}
		return c.ensureOCIApplicationState(ctx, spec)
	}
	if !errors.Is(err, hv.ErrGuestNotFound) {
		return hv.Guest{}, err
	}

	request := struct {
		Name   string            `json:"name"`
		Type   string            `json:"type"`
		Source map[string]string `json:"source"`
		Config map[string]string `json:"config"`
	}{
		Name: spec.Name,
		Type: "container",
		Source: map[string]string{
			"type": "image", "mode": "pull", "protocol": "oci",
			"server": image.Server, "alias": image.Alias,
		},
		Config: map[string]string{
			"limits.cpu":                          strconv.Itoa(spec.Cores),
			"limits.memory":                       strconv.Itoa(spec.MemoryMiB) + "MiB",
			"user.shippin.deployment_id":          spec.DeploymentID,
			"user.shippin.generation":             strconv.FormatInt(spec.Generation, 10),
			"user.shippin.image_reference_sha256": imageReferenceHash(spec.ImageReference),
		},
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return hv.Guest{}, err
	}
	body, code, err := c.do(ctx, "POST", c.path("/1.0/instances"), raw)
	if err != nil {
		return hv.Guest{}, err
	}
	if code != 200 && code != 201 && code != 202 {
		return hv.Guest{}, fmt.Errorf("incus: OCI application create HTTP %d: %s", code, clip(body))
	}
	if err := c.wait(ctx, body); err != nil {
		return hv.Guest{}, fmt.Errorf("incus: OCI application create operation failed: %w", err)
	}
	return c.ensureOCIApplicationState(ctx, spec)
}

func (c *Client) ensureOCIApplicationState(ctx context.Context, spec OCIApplicationSpec) (hv.Guest, error) {
	instance, err := c.instanceDetails(ctx, spec.Name)
	if err != nil {
		return hv.Guest{}, err
	}
	if err := validateOCIApplicationIdentity(instance, spec); err != nil {
		return hv.Guest{}, err
	}
	if spec.Start && !strings.EqualFold(instance.Status, "Running") {
		if _, err := c.Start(ctx, spec.Name); err != nil {
			return hv.Guest{}, fmt.Errorf("incus: OCI application was created but did not start: %w", err)
		}
	} else if !spec.Start && !strings.EqualFold(instance.Status, "Stopped") {
		if _, err := c.Stop(ctx, spec.Name); err != nil {
			return hv.Guest{}, fmt.Errorf("incus: OCI application was created but did not stop: %w", err)
		}
	}
	instance, err = c.instanceDetails(ctx, spec.Name)
	if err != nil {
		return hv.Guest{}, err
	}
	if err := validateOCIApplicationIdentity(instance, spec); err != nil {
		return hv.Guest{}, err
	}
	want := "Stopped"
	if spec.Start {
		want = "Running"
	}
	if !strings.EqualFold(instance.Status, want) {
		return hv.Guest{}, fmt.Errorf("provider reports application state %q; wanted %q", instance.Status, want)
	}
	return instance.guest(), nil
}

func validateOCIApplicationIdentity(instance instance, spec OCIApplicationSpec) error {
	config := instance.Config
	if instance.Type != "container" || config["volatile.container.oci"] != "true" {
		return errors.New("provider read-back does not identify an Incus OCI application container")
	}
	if config["user.shippin.deployment_id"] != spec.DeploymentID ||
		config["user.shippin.generation"] != strconv.FormatInt(spec.Generation, 10) ||
		config["user.shippin.image_reference_sha256"] != imageReferenceHash(spec.ImageReference) {
		return errors.New("Incus instance name is owned by a different Shippin deployment generation or image")
	}
	if config["limits.cpu"] != strconv.Itoa(spec.Cores) || config["limits.memory"] != strconv.Itoa(spec.MemoryMiB)+"MiB" {
		return errors.New("Incus OCI application resource configuration does not match the requested generation")
	}
	return nil
}

func imageReferenceHash(reference string) string {
	digest := sha256.Sum256([]byte(reference))
	return hex.EncodeToString(digest[:])
}

func incusVersionAtLeast(version string, major, minor, patch int) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	values := [3]int{}
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return false
		}
		values[i] = value
	}
	if values[0] != major {
		return values[0] > major
	}
	if values[1] != minor {
		return values[1] > minor
	}
	return values[2] >= patch
}
