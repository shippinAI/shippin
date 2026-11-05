package incus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/hv"
)

// OCIApplicationSpec describes the deliberately small Incus OCI workload
// primitive, with an explicitly selected managed private network and optional
// deployment-owned persistent storage. It does not configure edge exposure,
// health checks, upgrades, backups, or removal; catalog deployments must not
// use it until those lifecycle contracts and worker authorization exist.
type OCIApplicationSpec struct {
	Name           string
	DeploymentID   string
	Generation     int64
	ImageReference string
	Network        string
	MemoryMiB      int
	Cores          int
	// Environment values are stored in Incus instance configuration and can be
	// read by authorized Incus administrators. Do not populate this from a
	// secret resolver until the installation's provider trust boundary has been
	// explicitly accepted.
	Environment    map[string]string
	StoragePool    string
	StorageSizeGiB int
	MountPath      string
	Start          bool
}

type ociImageReference struct {
	Server string
	Alias  string
}

var ociDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var ociEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
var ociNetworkName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

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
	image, err := validateOCIApplicationSpec(spec)
	if err != nil {
		return hv.Guest{}, err
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
		if spec.StoragePool != "" {
			if err := c.checkOCIApplicationVolume(ctx, spec); err != nil {
				return hv.Guest{}, err
			}
		}
		return c.ensureOCIApplicationState(ctx, spec)
	}
	if !errors.Is(err, hv.ErrGuestNotFound) {
		return hv.Guest{}, err
	}
	if spec.StoragePool != "" {
		if err := c.ensureOCIApplicationVolume(ctx, spec); err != nil {
			return hv.Guest{}, err
		}
	}

	request := struct {
		Name    string                       `json:"name"`
		Type    string                       `json:"type"`
		Source  map[string]string            `json:"source"`
		Config  map[string]string            `json:"config"`
		Devices map[string]map[string]string `json:"devices,omitempty"`
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
	for name, value := range spec.Environment {
		request.Config["environment."+name] = value
	}
	request.Devices = ociApplicationDevices(spec)
	raw, err := json.Marshal(request)
	if err != nil {
		return hv.Guest{}, err
	}
	body, code, err := c.do(ctx, "POST", c.path("/1.0/instances"), raw)
	if err != nil {
		return hv.Guest{}, err
	}
	if code != 200 && code != 201 && code != 202 {
		if len(spec.Environment) > 0 {
			return hv.Guest{}, fmt.Errorf("incus: OCI application create HTTP %d", code)
		}
		return hv.Guest{}, fmt.Errorf("incus: OCI application create HTTP %d: %s", code, clip(body))
	}
	if err := c.wait(ctx, body); err != nil {
		if len(spec.Environment) > 0 {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return hv.Guest{}, err
			}
			return hv.Guest{}, errors.New("incus: OCI application create operation failed")
		}
		return hv.Guest{}, fmt.Errorf("incus: OCI application create operation failed: %w", err)
	}
	return c.ensureOCIApplicationState(ctx, spec)
}

// validateOCIApplicationSpec applies the same identity and provider bounds to
// every lifecycle action. The returned image has already passed immutable
// digest validation and is safe to use in ownership comparisons.
func validateOCIApplicationSpec(spec OCIApplicationSpec) (ociImageReference, error) {
	if !guestName.MatchString(spec.Name) {
		return ociImageReference{}, errors.New("incus: application name starts with a letter and uses letters, digits and dashes")
	}
	if _, err := uuid.Parse(spec.DeploymentID); err != nil || spec.Generation < 1 {
		return ociImageReference{}, errors.New("incus: application requires a deployment UUID and positive generation")
	}
	image, err := parseOCIApplicationReference(spec.ImageReference)
	if err != nil {
		return ociImageReference{}, err
	}
	if !ociNetworkName.MatchString(spec.Network) || strings.TrimSpace(spec.Network) != spec.Network {
		return ociImageReference{}, errors.New("incus: application requires an explicitly selected managed network")
	}
	if spec.Cores < 1 || spec.Cores > 32 || spec.MemoryMiB < 128 || spec.MemoryMiB > 65536 {
		return ociImageReference{}, errors.New("incus: application resources must be 1-32 cores and 128-65536 MiB memory")
	}
	if err := validateOCIApplicationStorage(spec); err != nil {
		return ociImageReference{}, err
	}
	if len(spec.Environment) > 16 {
		return ociImageReference{}, errors.New("incus: OCI application supports at most 16 environment values")
	}
	totalEnvironmentBytes := 0
	for name, value := range spec.Environment {
		if !ociEnvironmentName.MatchString(name) || strings.ContainsRune(value, '\x00') || len(value) > 8192 {
			return ociImageReference{}, errors.New("incus: OCI application environment name or value is invalid")
		}
		totalEnvironmentBytes += len(name) + len(value)
		if totalEnvironmentBytes > 65536 {
			return ociImageReference{}, errors.New("incus: OCI application environment exceeds 64 KiB")
		}
	}
	return image, nil
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
	for key, value := range config {
		if strings.HasPrefix(key, "environment.") {
			if expected, ok := spec.Environment[strings.TrimPrefix(key, "environment.")]; !ok || expected != value {
				return errors.New("Incus OCI application environment does not match the requested generation")
			}
		}
	}
	for name, value := range spec.Environment {
		if config["environment."+name] != value {
			return errors.New("Incus OCI application environment does not match the requested generation")
		}
	}
	if spec.StoragePool != "" {
		device, ok := instance.Devices["shippin-data"]
		if !ok || device["type"] != "disk" || device["pool"] != spec.StoragePool ||
			device["source"] != ociApplicationVolumeName(spec.DeploymentID) || device["path"] != spec.MountPath {
			return errors.New("Incus OCI application persistent storage does not match the requested deployment")
		}
	}
	network, ok := instance.Devices["eth0"]
	if !ok || network["type"] != "nic" || network["name"] != "eth0" || network["network"] != spec.Network {
		return errors.New("Incus OCI application private network does not match the requested target network")
	}
	return nil
}

func validateOCIApplicationStorage(spec OCIApplicationSpec) error {
	if spec.StoragePool == "" && spec.StorageSizeGiB == 0 && spec.MountPath == "" {
		return nil
	}
	if !guestName.MatchString(spec.StoragePool) || spec.StorageSizeGiB < 1 || spec.StorageSizeGiB > 65_536 ||
		!strings.HasPrefix(spec.MountPath, "/") || spec.MountPath == "/" || strings.ContainsAny(spec.MountPath, "\\\r\n\x00") || path.Clean(spec.MountPath) != spec.MountPath {
		return errors.New("incus: persistent storage requires a valid configured pool, size, and absolute mount path")
	}
	return nil
}

func ociApplicationVolumeName(deploymentID string) string {
	return "shippin-" + strings.ReplaceAll(deploymentID, "-", "")
}

func ociApplicationDevices(spec OCIApplicationSpec) map[string]map[string]string {
	devices := map[string]map[string]string{
		"eth0": {"type": "nic", "name": "eth0", "network": spec.Network},
	}
	if spec.StoragePool != "" {
		devices["shippin-data"] = map[string]string{
			"type": "disk", "pool": spec.StoragePool, "source": ociApplicationVolumeName(spec.DeploymentID),
			"path": spec.MountPath,
		}
	}
	return devices
}

func (c *Client) ensureOCIApplicationVolume(ctx context.Context, spec OCIApplicationSpec) error {
	poolPath := "/1.0/storage-pools/" + url.PathEscape(spec.StoragePool) + "/volumes/custom"
	volumePath := poolPath + "/" + url.PathEscape(ociApplicationVolumeName(spec.DeploymentID))
	volume, code, err := c.do(ctx, "GET", c.path(volumePath), nil)
	if err != nil {
		return err
	}
	if code == http.StatusOK {
		return validateOCIApplicationVolume(volume, spec)
	}
	if code != http.StatusNotFound {
		return fmt.Errorf("incus: persistent volume lookup HTTP %d: %s", code, clip(volume))
	}
	payload, err := json.Marshal(struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Config      map[string]string `json:"config"`
	}{ociApplicationVolumeName(spec.DeploymentID), ociApplicationVolumeDescription(spec.DeploymentID),
		map[string]string{"size": strconv.Itoa(spec.StorageSizeGiB) + "GiB"}})
	if err != nil {
		return err
	}
	body, code, err := c.do(ctx, "POST", c.path(poolPath), payload)
	if err != nil {
		return err
	}
	if code == http.StatusConflict {
		body, code, err = c.do(ctx, "GET", c.path(volumePath), nil)
		if err != nil {
			return err
		}
		if code != http.StatusOK {
			return fmt.Errorf("incus: concurrent persistent volume creation could not be read back (HTTP %d)", code)
		}
		return validateOCIApplicationVolume(body, spec)
	}
	if code != http.StatusOK && code != http.StatusCreated && code != http.StatusAccepted {
		return fmt.Errorf("incus: persistent volume create HTTP %d: %s", code, clip(body))
	}
	if err := c.wait(ctx, body); err != nil {
		return fmt.Errorf("incus: persistent volume create operation failed: %w", err)
	}
	body, code, err = c.do(ctx, "GET", c.path(volumePath), nil)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("incus: persistent volume read-back HTTP %d: %s", code, clip(body))
	}
	return validateOCIApplicationVolume(body, spec)
}

func (c *Client) checkOCIApplicationVolume(ctx context.Context, spec OCIApplicationSpec) error {
	volumePath := "/1.0/storage-pools/" + url.PathEscape(spec.StoragePool) + "/volumes/custom/" + url.PathEscape(ociApplicationVolumeName(spec.DeploymentID))
	body, code, err := c.do(ctx, "GET", c.path(volumePath), nil)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("incus: persistent volume is missing or unavailable (HTTP %d): %s", code, clip(body))
	}
	return validateOCIApplicationVolume(body, spec)
}

func ociApplicationVolumeDescription(deploymentID string) string {
	return "Shippin deployment " + deploymentID
}

func validateOCIApplicationVolume(body []byte, spec OCIApplicationSpec) error {
	var response struct {
		Metadata struct {
			Name        string            `json:"name"`
			Description string            `json:"description"`
			Config      map[string]string `json:"config"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return errors.New("incus: persistent volume read-back is invalid")
	}
	if response.Metadata.Name != ociApplicationVolumeName(spec.DeploymentID) ||
		response.Metadata.Description != ociApplicationVolumeDescription(spec.DeploymentID) ||
		response.Metadata.Config["size"] != strconv.Itoa(spec.StorageSizeGiB)+"GiB" {
		return errors.New("incus: persistent volume name is already owned by a different configuration")
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
