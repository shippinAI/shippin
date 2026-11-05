package incus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"shippin.cloud/console/internal/hv"
)

// OCIApplicationSpec describes the deliberately small, stateless Incus OCI
// workload primitive. It does not configure credentials, persistent data,
// network exposure, health checks, or upgrades; catalog deployments must not
// use it until those lifecycle contracts are implemented as well.
type OCIApplicationSpec struct {
	Name           string
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
	if _, err := c.Guest(ctx, spec.Name); err == nil {
		return hv.Guest{}, fmt.Errorf("incus: guest %q already exists", spec.Name)
	} else if !errors.Is(err, hv.ErrGuestNotFound) {
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
			"limits.cpu":    strconv.Itoa(spec.Cores),
			"limits.memory": strconv.Itoa(spec.MemoryMiB) + "MiB",
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
	if spec.Start {
		_, err := c.Start(ctx, spec.Name)
		if err != nil {
			return hv.Guest{}, fmt.Errorf("incus: OCI application was created but did not start: %w", err)
		}
	}
	created, err := c.applicationReadback(ctx, spec.Name, spec.Start)
	if err != nil {
		return hv.Guest{}, fmt.Errorf("incus: OCI application was created but read-back failed: %w", err)
	}
	return created, nil
}

func (c *Client) applicationReadback(ctx context.Context, name string, started bool) (hv.Guest, error) {
	body, code, err := c.do(ctx, http.MethodGet, c.path(instancePath(name)), nil)
	if err != nil {
		return hv.Guest{}, err
	}
	if code != http.StatusOK {
		return hv.Guest{}, fmt.Errorf("incus: application read-back HTTP %d: %s", code, clip(body))
	}
	var result struct {
		Metadata instance `json:"metadata"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return hv.Guest{}, err
	}
	if result.Metadata.Type != "container" || result.Metadata.Config["volatile.container.oci"] != "true" {
		return hv.Guest{}, errors.New("provider read-back does not identify an Incus OCI application container")
	}
	if started && !strings.EqualFold(result.Metadata.Status, "Running") {
		return hv.Guest{}, fmt.Errorf("provider reports application state %q after start", result.Metadata.Status)
	}
	return result.Metadata.guest(), nil
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
